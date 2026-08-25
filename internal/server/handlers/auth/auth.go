package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	"github.com/gougoujiang/buildmax/internal/server/access"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/util"
)

// LoginRequest is the JSON body for POST /api/login. Exactly one of Password
// and Otp is used; Password is tried first when both arrive.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Otp      string `json:"otp"`
	Platform string `json:"platform"`
}

// POST /api/login accepts two credentials, for two different jobs.
//
//  1. A password, which the person chose and can use every day. This is the
//     ordinary way in.
//
//  2. A single-use login code from Config.LoginCodeStore, issued out of band
//     with `buildmax-server user login-code <email>` and delivered by the
//     operator. It belongs to one account, expires, and cannot be replayed.
//     BuildMax has no mail channel, so this is the recovery path: it is how a
//     new account is claimed and how a forgotten password is reset. Signing in
//     with a code and then setting a password is the whole flow.
//
// The two are not tiers of the same thing. A password is what someone knows; a
// code is what an operator vouched for. Either produces the same session.

// LoginResponse is the JSON body for a successful login.
type LoginResponse struct {
	// Token is AccessToken under its original name. It predates the split into
	// two credentials and is still populated so that a client written against
	// the single-token response keeps working.
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	// RefreshToken is empty when the deployment has no store to keep it in —
	// see issueTokenPair.
	RefreshToken string `json:"refresh_token,omitempty"`
	// ExpiresIn is the access token's lifetime in seconds. Clients refresh on
	// it rather than parsing the JWT.
	ExpiresIn int64     `json:"expires_in"`
	User      LoginUser `json:"user"`
}

// RefreshRequest is the JSON body for POST /api/token/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshResponse is the JSON body for a successful refresh.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// LogoutRequest is the JSON body for POST /api/logout. The refresh token is
// optional: a caller that no longer has one falls back to the session named by
// its access token.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LoginUser is the user subset returned in the login response.
type LoginUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// OtpRequestRequest is the JSON body for POST /api/otp/request.
type OtpRequestRequest struct {
	Email  string `json:"email"`
	Intent string `json:"intent"` // "signup" or "login"; default "signup"
}

// OtpRequestResponse is the JSON body for a successful account request.
//
// Message is "account_created" or "account_exists". It deliberately no longer
// says "otp_sent": nothing was sent, and no client should tell someone to check
// an inbox that will stay empty.
type OtpRequestResponse struct {
	Message string `json:"message"`
}

func (h *Handler) accessTokenTTL() time.Duration {
	if h.cfg.AccessTokenTTL > 0 {
		return h.cfg.AccessTokenTTL
	}
	return coreidentity.AccessTokenTTLDefault
}

func (h *Handler) refreshTokenTTL() time.Duration {
	if h.cfg.RefreshTokenTTL > 0 {
		return h.cfg.RefreshTokenTTL
	}
	return coreidentity.RefreshTokenTTLDefault
}

func (h *Handler) refreshRotationGrace() time.Duration {
	if h.cfg.RefreshRotationGrace > 0 {
		return h.cfg.RefreshRotationGrace
	}
	return coreidentity.RefreshRotationGraceDefault
}

// issueTokenPair signs an access token for sessionID and, when the deployment
// has a store for it, issues the refresh token that will replace it.
//
// A deployment without a RefreshTokenStore still logs people in; they simply
// get one credential that expires and has to be earned again with a new login
// code. That is exactly the behavior BuildMax had before refresh tokens
// existed, which makes the store optional rather than required.
func (h *Handler) issueTokenPair(ctx context.Context, userID, platform, sessionID string, now time.Time) (accessToken, refreshToken string, expiresIn int64, err error) {
	ttl := h.accessTokenTTL()
	accessToken, err = access.Mint(h.cfg.JWTSecret, userID, sessionID, now, ttl)
	if err != nil {
		return "", "", 0, err
	}
	if h.cfg.RefreshTokens != nil {
		refreshToken, _, err = h.cfg.RefreshTokens.CreateRefreshToken(ctx, coreidentity.NewRefreshToken{
			UserID:    userID,
			SessionID: sessionID,
			Platform:  platform,
			TTL:       h.refreshTokenTTL(),
		})
		if err != nil {
			return "", "", 0, err
		}
	}
	return accessToken, refreshToken, int64(ttl.Seconds()), nil
}

func (h *Handler) loginHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Users == nil || h.cfg.JWTSecret == "" {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login not configured")
		return
	}
	if h.cfg.LoginCodes == nil && h.cfg.Passwords == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login not configured: no way to verify a credential")
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Both of these arrive by paste, and both used to fail on whitespace the
	// person could not see: a login code copied out of a terminal carries the
	// indentation of the line it was printed on, and an autofilled address can
	// carry a trailing space. The password is left alone — whitespace in one
	// may be deliberate.
	req.Email = strings.TrimSpace(req.Email)
	req.Otp = strings.TrimSpace(req.Otp)
	if req.Email == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "email required")
		return
	}
	if req.Password == "" && req.Otp == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "password or otp required")
		return
	}

	var user *coreidentity.User
	var method string
	var ok bool
	if req.Password != "" {
		user, ok = h.verifyPassword(w, r, req)
		method = loginMethodPassword
	} else {
		user, ok = h.verifyLoginCode(w, r, req)
		method = loginMethodLoginCode
	}
	if !ok {
		return
	}
	// Checked after the credential verifies, never before. Refusing a disabled
	// account earlier would answer an unauthenticated caller "that address is
	// registered but switched off", which is more than a wrong password is
	// told. Someone who just proved the account is theirs, on the other hand,
	// should hear the real reason rather than "wrong password".
	if !h.refuseDisabled(w, r, user, "login") {
		return
	}
	now := time.Now()
	platform := req.Platform
	if platform == "" {
		platform = "unknown"
	}
	// Every login opens its own session. Signing in from a second machine
	// therefore does not disturb the first, and revoking one leaves the other
	// alone — which is the whole point of tracking sessions rather than users.
	sessionID, err := util.NewPublicID()
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "session_id")
		return
	}
	accessToken, refreshToken, expiresIn, err := h.issueTokenPair(r.Context(), user.ID, platform, sessionID, now)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "issue_token_pair")
		return
	}
	if err := h.cfg.Users.UpdateLoginMeta(r.Context(), user.ID, now, platform); err != nil {
		slog.Error("update login meta failed", "err", err, "handler", "login", "user_id", user.ID)
	}
	// Recorded after the login succeeds, so the trail holds sessions that were
	// actually issued rather than attempts. A login has no team.
	h.cfg.Audit.Record(r.Context(), coreaudit.Event{
		ActorType:  coreaudit.ActorUser,
		ActorID:    user.ID,
		Action:     coreaudit.UserLogin,
		TargetType: "platform",
		TargetID:   platform,
		// Which credential authenticated. A trail that cannot tell a password
		// from an operator-issued code cannot answer who let someone in, and
		// adding the field afterwards would leave every earlier row unable to
		// say.
		Detail: method,
	})
	httputil.WriteJSON(w, http.StatusOK, LoginResponse{
		Token:        accessToken,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
		User:         LoginUser{ID: user.ID, Email: user.Email, Name: user.Name},
	})
}

// refreshHandler exchanges a refresh token for a new access token and the next
// refresh token.
//
// It is unauthenticated by design: possession of the refresh token is the
// proof, and requiring a live access token alongside it would make the endpoint
// useless in the one situation it exists for.
func (h *Handler) refreshHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Users == nil || h.cfg.JWTSecret == "" {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login not configured")
		return
	}
	if h.cfg.RefreshTokens == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "refresh not configured")
		return
	}
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RefreshToken == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "refresh_token required")
		return
	}
	now := time.Now()
	rotated, err := h.cfg.RefreshTokens.RotateRefreshToken(
		r.Context(), req.RefreshToken, now, h.refreshTokenTTL(), h.refreshRotationGrace())
	switch {
	case errors.Is(err, coreidentity.ErrRefreshTokenReused):
		// The store has already revoked the session. Record it: this is the
		// one signal a deployment gets that a credential was copied, and it
		// arrives without anyone reporting anything.
		h.cfg.Audit.Record(r.Context(), coreaudit.Event{
			ActorType:  coreaudit.ActorSystem,
			ActorID:    rotated.UserID,
			Action:     coreaudit.RefreshReuse,
			TargetType: "auth_session",
			TargetID:   rotated.SessionID,
		})
		slog.Warn("refresh token reused; session revoked",
			"handler", "refresh", "user_id", rotated.UserID, "session_id", rotated.SessionID)
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	case errors.Is(err, coreidentity.ErrRefreshTokenInvalid):
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	case err != nil:
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "refresh", "rotate")
		return
	}

	// The refresh token outlives many access tokens, so the account behind it
	// is re-checked here rather than trusted from the login it descends from.
	user, err := h.cfg.Users.GetUser(r.Context(), rotated.UserID)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "refresh", "user_id", rotated.UserID)
		return
	}
	if user == nil {
		if _, err := h.cfg.RefreshTokens.RevokeSession(r.Context(), rotated.SessionID, now); err != nil {
			slog.Error("revoke session for missing user failed", "err", err, "session_id", rotated.SessionID)
		}
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	// A disabled account's sessions are revoked when it is disabled, so a
	// refresh token that still rotates was issued after that or escaped the
	// sweep. Either way the account is the authority, not the row: revoke the
	// session this one belongs to and say why.
	if user.Disabled() {
		if _, err := h.cfg.RefreshTokens.RevokeSession(r.Context(), rotated.SessionID, now); err != nil {
			slog.Error("revoke session for disabled user failed", "err", err, "session_id", rotated.SessionID)
		}
		httputil.WriteJSONError(w, http.StatusForbidden, access.DisabledMessage)
		return
	}

	ttl := h.accessTokenTTL()
	accessToken, err := access.Mint(h.cfg.JWTSecret, user.ID, rotated.SessionID, now, ttl)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "refresh", "sign_token")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, RefreshResponse{
		AccessToken:  accessToken,
		RefreshToken: rotated.Plaintext,
		ExpiresIn:    int64(ttl.Seconds()),
	})
}

// SetPasswordRequest is the JSON body for POST /api/password.
type SetPasswordRequest struct {
	// CurrentPassword is required when the account already has one. It is not
	// used when setting the first password, because there is nothing to prove
	// against and the caller already holds a session.
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// setPasswordHandler sets or changes the caller's password.
//
// Changing an existing password requires the current one. The session alone is
// not enough: a stolen access token cannot be revoked before it expires, and
// letting one set a password would turn a temporary theft into a permanent
// account takeover. Setting the *first* password does run on the session
// alone — that session came from a login code an operator issued by hand, which
// is the strongest proof this deployment has.
func (h *Handler) setPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Passwords == nil || h.cfg.Users == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "password login is not configured")
		return
	}
	userID, ok := h.guard().ActiveUser(w, r)
	if !ok {
		return
	}
	var req SetPasswordRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if err := coreidentity.ValidatePassword(req.NewPassword); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := h.cfg.Passwords.PasswordHash(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "set_password", "password_hash")
		return
	}
	if existing != "" && !coreidentity.VerifyPassword(existing, req.CurrentPassword) {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := coreidentity.HashPassword(req.NewPassword)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.cfg.Passwords.SetPassword(r.Context(), userID, hash, time.Now().UTC()); err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "set_password", "user_id", userID)
		return
	}
	h.cfg.Audit.Record(r.Context(), coreaudit.Event{
		ActorType:  coreaudit.ActorUser,
		ActorID:    userID,
		Action:     coreaudit.PasswordSet,
		TargetType: "user",
		TargetID:   userID,
	})
	w.WriteHeader(http.StatusNoContent)
}

// logoutHandler revokes one session.
//
// It accepts either credential. A refresh token names its session directly; an
// access token carries the session in its sid claim. A caller holding neither
// has nothing to revoke, and clearing its own state is all that is left.
func (h *Handler) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.RefreshTokens == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "refresh not configured")
		return
	}
	var req LogoutRequest
	if r.Body != nil {
		// An empty body is normal here — the access token alone is enough.
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	now := time.Now().UTC()

	var userID, sessionID string
	if req.RefreshToken != "" {
		var err error
		userID, sessionID, err = h.cfg.RefreshTokens.RevokeRefreshTokenSession(r.Context(), req.RefreshToken, now)
		if err != nil {
			httputil.WriteInternalError(w, err, "auth handler error", "handler", "logout", "revoke_by_token")
			return
		}
	} else {
		claims, ok := access.ClaimsFromRequest(r, h.cfg.JWTSecret)
		if !ok || claims.Sid == "" {
			httputil.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		userID, sessionID = claims.Sub, claims.Sid
		if _, err := h.cfg.RefreshTokens.RevokeSession(r.Context(), sessionID, now); err != nil {
			httputil.WriteInternalError(w, err, "auth handler error", "handler", "logout", "revoke_session")
			return
		}
	}

	// An unknown token revokes nothing and still answers 204: a client should
	// be able to log out of a session the server has already forgotten.
	if sessionID != "" {
		h.cfg.Audit.Record(r.Context(), coreaudit.Event{
			ActorType:  coreaudit.ActorUser,
			ActorID:    userID,
			Action:     coreaudit.UserLogout,
			TargetType: "auth_session",
			TargetID:   sessionID,
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// Login methods, as recorded in the audit trail.
const (
	loginMethodPassword  = "password"
	loginMethodLoginCode = "login_code"
)

// invalidPasswordMessage is the single answer to every failed password login.
//
// An unknown address, an account with no password set, and a wrong password
// are all the same sentence. Telling them apart would turn the login form into
// a way to ask which addresses have accounts here.
const invalidPasswordMessage = "invalid email or password"

// verifyPassword resolves the user a submitted password authenticates.
func (h *Handler) verifyPassword(w http.ResponseWriter, r *http.Request, req LoginRequest) (*coreidentity.User, bool) {
	if h.cfg.Passwords == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "password login is not configured")
		return nil, false
	}
	user, err := h.cfg.Users.UserByEmail(r.Context(), req.Email)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "email", req.Email)
		return nil, false
	}

	var hash string
	if user != nil {
		hash, err = h.cfg.Passwords.PasswordHash(r.Context(), user.ID)
		if err != nil {
			httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "password_hash")
			return nil, false
		}
	}
	if hash == "" {
		// No account, or one that has never set a password. Hash anyway: the
		// work is what makes the two cases take the same time, and time is the
		// other channel that would answer "is this address registered".
		coreidentity.DummyVerifyPassword(req.Password)
		httputil.WriteJSONError(w, http.StatusUnauthorized, invalidPasswordMessage)
		return nil, false
	}
	if !coreidentity.VerifyPassword(hash, req.Password) {
		httputil.WriteJSONError(w, http.StatusUnauthorized, invalidPasswordMessage)
		return nil, false
	}
	return user, true
}

// verifyLoginCode resolves the user the submitted code authenticates, writing
// the response and returning ok=false when it does not authenticate anyone.
//
// The account is resolved from the submitted address first, and the code is
// redeemed only if it was issued to that account. The order matters: redeeming
// first and comparing the address afterwards spent the code on a typo, so the
// operator had to issue another one and the person retrying with the right
// address was refused for a reason neither of them could see.
//
// Single use is unaffected. Redemption is still one conditional UPDATE, so two
// browsers submitting the same code with the right address still produce
// exactly one session.
func (h *Handler) verifyLoginCode(w http.ResponseWriter, r *http.Request, req LoginRequest) (*coreidentity.User, bool) {
	if h.cfg.LoginCodes == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login codes are not configured")
		return nil, false
	}
	user, err := h.cfg.Users.UserByEmail(r.Context(), req.Email)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "email", req.Email)
		return nil, false
	}
	// The reason a sign-in failed is deliberately absent from the response and
	// present in the log. The operator debugging it is on this side of the
	// server, and a 401 line was all they had: an unknown address, a spent
	// code, and a code pasted into the wrong browser were one sentence.
	if user == nil {
		slog.InfoContext(r.Context(), "login code rejected", "reason", "no account for that email", "email", req.Email)
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid otp")
		return nil, false
	}
	redeemed, err := h.cfg.LoginCodes.ConsumeLoginCode(r.Context(), req.Otp, user.ID, time.Now().UTC())
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "stage", "consume_login_code")
		return nil, false
	}
	if !redeemed {
		slog.InfoContext(r.Context(), "login code rejected",
			"reason", "unknown, spent, expired, or issued to another account", "user_id", user.ID)
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid otp")
		return nil, false
	}
	return user, true
}

func (h *Handler) otpRequestHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Users == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "otp not configured")
		return
	}
	var req OtpRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "email required")
		return
	}
	intent := req.Intent
	if intent == "" {
		intent = "signup"
	}
	if intent != "signup" && intent != "login" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "intent must be signup or login")
		return
	}
	user, err := h.cfg.Users.UserByEmail(r.Context(), req.Email)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if intent == "login" {
		if user == nil {
			httputil.WriteJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		// The account exists. Nothing is sent, because there is nothing to send
		// it with; getting in means a password, or a code an operator issues by
		// hand.
		httputil.WriteJSON(w, http.StatusOK, OtpRequestResponse{Message: "account_exists"})
		return
	}
	// intent == "signup"
	//
	// Closed by default. Nothing here verifies that whoever typed an address
	// controls it, so open registration on a reachable server is how someone
	// claims a colleague's address. Accounts are created by an operator
	// instead, with `buildmax-server user create`.
	if !h.cfg.AllowSignup {
		httputil.WriteJSONError(w, http.StatusForbidden,
			"signup is disabled on this server; ask an administrator for an account")
		return
	}
	if user != nil {
		httputil.WriteJSONError(w, http.StatusConflict, "email already registered")
		return
	}
	_, err = h.cfg.Users.CreateUser(r.Context(), req.Email, h.cfg.DefaultQuotaTier)
	if err != nil {
		if errors.Is(err, coreidentity.ErrEmailExists) {
			httputil.WriteJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		httputil.WriteJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// The account exists now, and still cannot be used: it has no password, and
	// no code has been issued for it. An operator has to run
	// `buildmax-server user login-code` before anyone can sign in — which is why
	// no BuildMax client offers a sign-up form.
	httputil.WriteJSON(w, http.StatusOK, OtpRequestResponse{Message: "account_created"})
}
