package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/util"

	"github.com/golang-jwt/jwt/v5"
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

type jwtClaims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
	// Typ separates an access token from anything else this key might ever
	// sign. Refresh tokens are opaque random strings rather than JWTs, so today
	// there is nothing to confuse an access token with; the claim is here so
	// that stays true if that ever changes.
	Typ string `json:"typ,omitempty"`
	// Sid names the login chain this token belongs to, matching session_id in
	// user_refresh_token. It is what lets a logout revoke the right session.
	Sid string `json:"sid,omitempty"`
}

const tokenTypeAccess = "access"

// jwtLeeway absorbs clock skew between the server that signed a token and the
// one validating it. A deployment running more than one replica has no
// guarantee their clocks agree to the second.
const jwtLeeway = 30 * time.Second

func (h *Handler) accessTokenTTL() time.Duration {
	if h.cfg.AccessTokenTTL > 0 {
		return h.cfg.AccessTokenTTL
	}
	return model.AccessTokenTTLDefault
}

func (h *Handler) refreshTokenTTL() time.Duration {
	if h.cfg.RefreshTokenTTL > 0 {
		return h.cfg.RefreshTokenTTL
	}
	return model.RefreshTokenTTLDefault
}

func (h *Handler) refreshRotationGrace() time.Duration {
	if h.cfg.RefreshRotationGrace > 0 {
		return h.cfg.RefreshRotationGrace
	}
	return model.RefreshRotationGraceDefault
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
	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        util.NewPrefixedID(util.PrefixAuthSession),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sub: userID,
		Typ: tokenTypeAccess,
		Sid: sessionID,
	}
	accessToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		return "", "", 0, err
	}
	if h.cfg.RefreshTokenStore != nil {
		refreshToken, _, err = h.cfg.RefreshTokenStore.CreateRefreshToken(ctx, model.NewRefreshToken{
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
	if h.cfg.UserStore == nil || h.cfg.JWTSecret == "" {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login not configured")
		return
	}
	if h.cfg.LoginCodeStore == nil && h.cfg.PasswordStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login not configured: no way to verify a credential")
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "email required")
		return
	}
	if req.Password == "" && req.Otp == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "password or otp required")
		return
	}

	var user *model.User
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
	now := time.Now()
	platform := req.Platform
	if platform == "" {
		platform = "unknown"
	}
	// Every login opens its own session. Signing in from a second machine
	// therefore does not disturb the first, and revoking one leaves the other
	// alone — which is the whole point of tracking sessions rather than users.
	sessionID := util.NewPrefixedID(util.PrefixAuthSession)
	accessToken, refreshToken, expiresIn, err := h.issueTokenPair(r.Context(), user.UserID, platform, sessionID, now)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "issue_token_pair")
		return
	}
	if err := h.cfg.UserStore.UpdateLoginMeta(r.Context(), user.UserID, now.Unix(), platform); err != nil {
		slog.Error("update login meta failed", "err", err, "handler", "login", "user_id", user.UserID)
	}
	// Recorded after the login succeeds, so the trail holds sessions that were
	// actually issued rather than attempts. A login has no team.
	h.cfg.Audit.Record(r.Context(), model.AuditEvent{
		ActorType:  model.AuditActorUser,
		ActorID:    user.UserID,
		Action:     model.AuditUserLogin,
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
		User:         LoginUser{ID: user.UserID, Email: user.Email, Name: user.Name},
	})
}

// refreshHandler exchanges a refresh token for a new access token and the next
// refresh token.
//
// It is unauthenticated by design: possession of the refresh token is the
// proof, and requiring a live access token alongside it would make the endpoint
// useless in the one situation it exists for.
func (h *Handler) refreshHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.UserStore == nil || h.cfg.JWTSecret == "" {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login not configured")
		return
	}
	if h.cfg.RefreshTokenStore == nil {
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
	rotated, err := h.cfg.RefreshTokenStore.RotateRefreshToken(
		r.Context(), req.RefreshToken, now.Unix(), h.refreshTokenTTL(), h.refreshRotationGrace())
	switch {
	case errors.Is(err, model.ErrRefreshTokenReused):
		// The store has already revoked the session. Record it: this is the
		// one signal a deployment gets that a credential was copied, and it
		// arrives without anyone reporting anything.
		h.cfg.Audit.Record(r.Context(), model.AuditEvent{
			ActorType:  model.AuditActorSystem,
			ActorID:    rotated.UserID,
			Action:     model.AuditRefreshReuse,
			TargetType: "auth_session",
			TargetID:   rotated.SessionID,
		})
		slog.Warn("refresh token reused; session revoked",
			"handler", "refresh", "user_id", rotated.UserID, "session_id", rotated.SessionID)
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	case errors.Is(err, model.ErrRefreshTokenInvalid):
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	case err != nil:
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "refresh", "rotate")
		return
	}

	// The refresh token outlives many access tokens, so the account behind it
	// is re-checked here rather than trusted from the login it descends from.
	user, err := h.cfg.UserStore.GetUser(r.Context(), rotated.UserID)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "refresh", "user_id", rotated.UserID)
		return
	}
	if user == nil {
		if _, err := h.cfg.RefreshTokenStore.RevokeSession(r.Context(), rotated.SessionID, now.Unix()); err != nil {
			slog.Error("revoke session for missing user failed", "err", err, "session_id", rotated.SessionID)
		}
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	ttl := h.accessTokenTTL()
	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        util.NewPrefixedID(util.PrefixAuthSession),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sub: user.UserID,
		Typ: tokenTypeAccess,
		Sid: rotated.SessionID,
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.cfg.JWTSecret))
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
	if h.cfg.PasswordStore == nil || h.cfg.UserStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "password login is not configured")
		return
	}
	userID, ok := requireAuth(w, r, h.cfg.JWTSecret)
	if !ok {
		return
	}
	var req SetPasswordRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := model.ValidatePassword(req.NewPassword); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := h.cfg.PasswordStore.PasswordHash(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "set_password", "password_hash")
		return
	}
	if existing != "" && !model.VerifyPassword(existing, req.CurrentPassword) {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := model.HashPassword(req.NewPassword)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.cfg.PasswordStore.SetPassword(r.Context(), userID, hash, time.Now().Unix()); err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "set_password", "user_id", userID)
		return
	}
	h.cfg.Audit.Record(r.Context(), model.AuditEvent{
		ActorType:  model.AuditActorUser,
		ActorID:    userID,
		Action:     model.AuditPasswordSet,
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
	if h.cfg.RefreshTokenStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "refresh not configured")
		return
	}
	var req LogoutRequest
	if r.Body != nil {
		// An empty body is normal here — the access token alone is enough.
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	now := time.Now().Unix()

	var userID, sessionID string
	if req.RefreshToken != "" {
		var err error
		userID, sessionID, err = h.cfg.RefreshTokenStore.RevokeRefreshTokenSession(r.Context(), req.RefreshToken, now)
		if err != nil {
			httputil.WriteInternalError(w, err, "auth handler error", "handler", "logout", "revoke_by_token")
			return
		}
	} else {
		claims, ok := claimsFromRequest(r, h.cfg.JWTSecret)
		if !ok || claims.Sid == "" {
			httputil.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		userID, sessionID = claims.Sub, claims.Sid
		if _, err := h.cfg.RefreshTokenStore.RevokeSession(r.Context(), sessionID, now); err != nil {
			httputil.WriteInternalError(w, err, "auth handler error", "handler", "logout", "revoke_session")
			return
		}
	}

	// An unknown token revokes nothing and still answers 204: a client should
	// be able to log out of a session the server has already forgotten.
	if sessionID != "" {
		h.cfg.Audit.Record(r.Context(), model.AuditEvent{
			ActorType:  model.AuditActorUser,
			ActorID:    userID,
			Action:     model.AuditUserLogout,
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
func (h *Handler) verifyPassword(w http.ResponseWriter, r *http.Request, req LoginRequest) (*model.User, bool) {
	if h.cfg.PasswordStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "password login is not configured")
		return nil, false
	}
	user, err := h.cfg.UserStore.UserByEmail(r.Context(), req.Email)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "email", req.Email)
		return nil, false
	}

	var hash string
	if user != nil {
		hash, err = h.cfg.PasswordStore.PasswordHash(r.Context(), user.UserID)
		if err != nil {
			httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "password_hash")
			return nil, false
		}
	}
	if hash == "" {
		// No account, or one that has never set a password. Hash anyway: the
		// work is what makes the two cases take the same time, and time is the
		// other channel that would answer "is this address registered".
		model.DummyVerifyPassword(req.Password)
		httputil.WriteJSONError(w, http.StatusUnauthorized, invalidPasswordMessage)
		return nil, false
	}
	if !model.VerifyPassword(hash, req.Password) {
		httputil.WriteJSONError(w, http.StatusUnauthorized, invalidPasswordMessage)
		return nil, false
	}
	return user, true
}

// verifyLoginCode resolves the user the submitted code authenticates, writing
// the response and returning ok=false when it does not authenticate anyone.
//
// The code is spent once redeemed, whether or not the rest of the request
// succeeds — that is what "single use" has to mean for a replay to be
// impossible.
func (h *Handler) verifyLoginCode(w http.ResponseWriter, r *http.Request, req LoginRequest) (*model.User, bool) {
	if h.cfg.LoginCodeStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login codes are not configured")
		return nil, false
	}
	userID, err := h.cfg.LoginCodeStore.ConsumeLoginCode(r.Context(), req.Otp, time.Now().Unix())
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "consume_login_code")
		return nil, false
	}
	if userID == "" {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid otp")
		return nil, false
	}
	user, err := h.cfg.UserStore.GetUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "user_id", userID)
		return nil, false
	}
	// The code already names the user; the email is checked so that signing in
	// as someone else cannot happen by pasting the wrong code into the wrong
	// browser.
	if user == nil || !strings.EqualFold(user.Email, req.Email) {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid otp")
		return nil, false
	}
	return user, true
}

func (h *Handler) otpRequestHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.UserStore == nil {
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
	user, err := h.cfg.UserStore.UserByEmail(r.Context(), req.Email)
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
	_, err = h.cfg.UserStore.CreateUser(r.Context(), req.Email, h.cfg.DefaultQuotaTier)
	if err != nil {
		if errors.Is(err, model.ErrEmailExists) {
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

// --- JWT auth middleware helpers ---

func requireAuth(w http.ResponseWriter, r *http.Request, jwtSecret string) (string, bool) {
	userID, ok := userIDFromRequest(r, jwtSecret)
	if !ok {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return userID, true
}

func (h *Handler) requireStore(w http.ResponseWriter, store interface{}, unavailableMessage string) bool {
	if store == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, unavailableMessage)
		return false
	}
	return true
}

func pathValueRequired(w http.ResponseWriter, r *http.Request, key string) (value string, ok bool) {
	value = r.PathValue(key)
	if value == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, key+" required")
		return "", false
	}
	return value, true
}

func (h *Handler) withUserAndStore(w http.ResponseWriter, r *http.Request, store interface{}, unavailableMsg string) (userID string, ok bool) {
	if !h.requireStore(w, store, unavailableMsg) {
		return "", false
	}
	userID, ok = requireAuth(w, r, h.cfg.JWTSecret)
	if !ok {
		return "", false
	}
	return userID, true
}

func (h *Handler) withUserPathTeamAndStore(w http.ResponseWriter, r *http.Request, store interface{}, unavailableMsg string) (userID, teamID string, ok bool) {
	userID, ok = h.withUserAndStore(w, r, store, unavailableMsg)
	if !ok {
		return "", "", false
	}
	if !h.requireStore(w, h.cfg.TeamStore, "teams not configured") {
		return "", "", false
	}
	teamID, ok = pathValueRequired(w, r, "team_id")
	if !ok {
		return "", "", false
	}
	_, resolvedTeamID, ok := h.withExplicitTeam(w, r, userID, teamID)
	if !ok {
		return "", "", false
	}
	return userID, resolvedTeamID, true
}

func (h *Handler) withExplicitTeam(w http.ResponseWriter, r *http.Request, userID, teamID string) (string, string, bool) {
	if !h.requireStore(w, h.cfg.TeamStore, "teams not configured") {
		return "", "", false
	}
	resolvedTeamID, ok := h.resolveTeamID(w, r, userID, teamID)
	if !ok {
		return "", "", false
	}
	return userID, resolvedTeamID, true
}

func (h *Handler) resolveTeamID(w http.ResponseWriter, r *http.Request, userID, explicitTeamID string) (string, bool) {
	if explicitTeamID == "" {
		team, err := h.cfg.TeamStore.GetPersonalTeamByUser(r.Context(), userID)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_current_team", "user_id", userID)
			return "", false
		}
		if team == nil {
			httputil.WriteJSONError(w, http.StatusForbidden, "team not found")
			return "", false
		}
		return team.TeamID, true
	}
	teams, err := h.cfg.TeamStore.ListTeamsByUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_current_team", "user_id", userID)
		return "", false
	}
	for _, team := range teams {
		if team.TeamID == explicitTeamID {
			return team.TeamID, true
		}
	}
	// Every team-scoped route passes through here, so this one call covers
	// reaching into a team you are not a member of, whatever the route.
	h.cfg.Audit.Denied(r.Context(), userID, explicitTeamID, r.Pattern)
	httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
	return "", false
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func userIDFromRequest(r *http.Request, jwtSecret string) (string, bool) {
	claims, ok := claimsFromRequest(r, jwtSecret)
	if !ok {
		return "", false
	}
	return claims.Sub, true
}

func claimsFromRequest(r *http.Request, jwtSecret string) (*jwtClaims, bool) {
	if jwtSecret == "" {
		return nil, false
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return nil, false
	}
	tokenStr := strings.TrimSpace(auth[len(prefix):])
	if tokenStr == "" {
		return nil, false
	}
	return parseAccessToken(tokenStr, jwtSecret)
}

func userIDFromToken(tokenStr string, jwtSecret string) (string, bool) {
	if jwtSecret == "" || tokenStr == "" {
		return "", false
	}
	claims, ok := parseAccessToken(strings.TrimSpace(tokenStr), jwtSecret)
	if !ok {
		return "", false
	}
	return claims.Sub, true
}

// parseAccessToken verifies a token and confirms it is an access token.
//
// An empty typ is accepted: tokens signed before the claim existed are still
// valid until they expire, and rejecting them would sign every user out at
// upgrade — which in a deployment with no email means an operator issuing a
// login code to each of them by hand.
func parseAccessToken(tokenStr, jwtSecret string) (*jwtClaims, bool) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithLeeway(jwtLeeway))
	if err != nil || !token.Valid {
		return nil, false
	}
	claims, ok := token.Claims.(*jwtClaims)
	if !ok || claims.Sub == "" {
		return nil, false
	}
	if claims.Typ != "" && claims.Typ != tokenTypeAccess {
		return nil, false
	}
	return claims, true
}
