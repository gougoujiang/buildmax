package auth

import (
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
	identitysvc "github.com/gougoujiang/buildmax/internal/service/identity"
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
	// see the identity service.
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

func (h *Handler) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := h.identityService().Login(r.Context(), identitysvc.LoginCmd{
		Email:    req.Email,
		Password: req.Password,
		Otp:      req.Otp,
		Platform: req.Platform,
	})
	if err != nil {
		h.writeLoginError(w, r, req, err)
		return
	}
	if result.LoginMetaErr != nil {
		slog.Error("update login meta failed", "err", result.LoginMetaErr, "handler", "login", "user_id", result.User.ID)
	}
	// Recorded after the login succeeds, so the trail holds sessions that were
	// actually issued rather than attempts. A login has no team.
	h.cfg.Audit.Record(r.Context(), coreaudit.Event{
		ActorType:  coreaudit.ActorUser,
		ActorID:    result.User.ID,
		Action:     coreaudit.UserLogin,
		TargetType: "platform",
		TargetID:   result.Platform,
		// Which credential authenticated. A trail that cannot tell a password
		// from an operator-issued code cannot answer who let someone in, and
		// adding the field afterwards would leave every earlier row unable to
		// say.
		Detail: result.Method,
	})
	httputil.WriteJSON(w, http.StatusOK, LoginResponse{
		Token:        result.AccessToken,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		User:         LoginUser{ID: result.User.ID, Email: result.User.Email, Name: result.User.Name},
	})
}

// writeLoginError answers a refused login exactly as the handler used to.
//
// The two 401s are written here rather than derived from a Kind because apierr
// has no unauthenticated one, and because what they say is a decision this
// endpoint owns: one message for every way a password fails, so the response
// cannot be read as an answer to "is this address registered". The reason is
// logged instead, which is where an operator debugging a 401 can use it.
func (h *Handler) writeLoginError(w http.ResponseWriter, r *http.Request, req LoginRequest, err error) {
	var invalid *identitysvc.InvalidCredential
	if errors.As(err, &invalid) {
		attrs := []any{"reason", invalid.Reason}
		if invalid.UserID != "" {
			attrs = append(attrs, "user_id", invalid.UserID)
		} else {
			attrs = append(attrs, "email", strings.TrimSpace(req.Email))
		}
		if invalid.Method == identitysvc.MethodLoginCode {
			slog.InfoContext(r.Context(), "login code rejected", attrs...)
			httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid otp")
			return
		}
		slog.InfoContext(r.Context(), "password login rejected", attrs...)
		httputil.WriteJSONError(w, http.StatusUnauthorized, invalidPasswordMessage)
		return
	}
	if errors.Is(err, identitysvc.ErrDisabled) {
		slog.InfoContext(r.Context(), "refused a disabled account",
			"handler", "login", "remote", r.RemoteAddr)
		httputil.WriteJSONError(w, http.StatusForbidden, access.DisabledMessage)
		return
	}
	if httputil.WriteServiceError(w, err) {
		return
	}
	httputil.WriteInternalError(w, err, "auth handler error", "handler", "login")
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
// invalidPasswordMessage is what every failed password login answers, whatever
// actually went wrong. Three refusals a caller can tell apart are three answers
// to "is this address registered".
const invalidPasswordMessage = "invalid email or password"

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
