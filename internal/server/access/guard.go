package access

import (
	"context"
	"net/http"

	coreidentity "github.com/icloudbb/buildmax/internal/core/identity"
	corespace "github.com/icloudbb/buildmax/internal/core/space"
	"github.com/icloudbb/buildmax/internal/server/httputil"
	"github.com/icloudbb/buildmax/internal/service/audit"
)

// DisabledMessage is what a disabled account is told, and it is deliberately
// specific: the person holding the credential has already proved it is theirs,
// and "wrong password" would send them to reset one that works.
const DisabledMessage = "account_disabled"

// Guard answers, for one request, who is calling and whether they may proceed.
//
// Every method writes the refusal itself and reports whether the caller may
// continue, so a route reads as a list of gates rather than a tree of error
// handling.
type Guard struct {
	JWTSecret string
	Users     coreidentity.UserStore
	Spaces    corespace.Store
	Grants    coreidentity.SystemGrantStore
	// Audit records refusals. Nil discards them, which is what a deployment
	// without a database has.
	Audit *audit.Recorder
}

// ActiveUser authenticates the caller and refuses a disabled account.
//
// This is where "disable this account" becomes immediate. The access token is a
// signed JWT the server never stores, so it cannot be retired -- the only way
// to stop honouring one is to check where the identity is resolved, and this is
// the single funnel every authenticated route reaches. The cost is one
// primary-key read per request, strictly less than the ListSpaceMembers every
// space-scoped route already does. Waiting out the token instead would make
// "disable" mean "in about a week", which is not the feature.
func (g *Guard) ActiveUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := UserIDFromRequest(r, g.JWTSecret)
	if !ok {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	if g.Users == nil {
		// No store to ask. A deployment without one has no accounts to
		// disable, so there is nothing this check could have found.
		return userID, true
	}
	user, err := g.Users.GetUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "require_active_user", "user_id", userID)
		return "", false
	}
	// A token naming an account the store does not have is allowed through
	// unchanged. Nothing deletes accounts, so this is not a state a deployment
	// reaches; tightening it is a separate decision from disablement, and
	// making it here would change what an unknown subject means on every route
	// at once.
	if user != nil && user.Disabled() {
		httputil.WriteJSONError(w, http.StatusForbidden, DisabledMessage)
		return "", false
	}
	return userID, true
}

// UserAndStore authenticates the caller and refuses when the feature's store is
// absent.
func (g *Guard) UserAndStore(w http.ResponseWriter, r *http.Request, store any, unavailable string) (string, bool) {
	if !httputil.RequireStore(w, store, unavailable) {
		return "", false
	}
	return g.ActiveUser(w, r)
}

// UserAndPathSpace is the preamble of every space-scoped route: the store exists,
// the caller is active, and they are in the space the path names.
func (g *Guard) UserAndPathSpace(w http.ResponseWriter, r *http.Request, store any, unavailable string) (userID, spaceID string, ok bool) {
	userID, ok = g.UserAndStore(w, r, store, unavailable)
	if !ok {
		return "", "", false
	}
	spaceID, ok = httputil.PathValue(w, r, "space_id")
	if !ok {
		return "", "", false
	}
	_, resolved, ok := g.ExplicitSpace(w, r, userID, spaceID)
	if !ok {
		return "", "", false
	}
	return userID, resolved, true
}

// ExplicitSpace resolves the space a caller may act in, defaulting to their
// personal space when the path named none.
func (g *Guard) ExplicitSpace(w http.ResponseWriter, r *http.Request, userID, spaceID string) (string, string, bool) {
	if !httputil.RequireStore(w, g.Spaces, "spaces not configured") {
		return "", "", false
	}
	resolved, ok := g.resolveSpaceID(w, r, userID, spaceID)
	if !ok {
		return "", "", false
	}
	return userID, resolved, true
}

func (g *Guard) resolveSpaceID(w http.ResponseWriter, r *http.Request, userID, explicit string) (string, bool) {
	if explicit == "" {
		space, err := g.Spaces.GetPersonalSpaceByUser(r.Context(), userID)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_current_space", "user_id", userID)
			return "", false
		}
		if space == nil {
			httputil.WriteJSONError(w, http.StatusForbidden, "space not found")
			return "", false
		}
		return space.ID, true
	}
	spaces, err := g.Spaces.ListSpacesByUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_current_space", "user_id", userID)
		return "", false
	}
	for _, space := range spaces {
		if space.ID == explicit {
			return explicit, true
		}
	}
	httputil.WriteJSONError(w, http.StatusForbidden, "space not found")
	return "", false
}

// UserAndDefaultSpace authorizes a route that does not name a space in its path.
//
// It exists for clients that have a server but have not chosen a space: a CLI or
// Desktop session knows its login and nothing else. An explicit space_id is
// honoured and still checked for membership; an empty one resolves to the
// caller's personal space, which is the private single-user case the product
// already represents that way.
func (g *Guard) UserAndDefaultSpace(w http.ResponseWriter, r *http.Request, explicitSpaceID string) (userID, spaceID string, ok bool) {
	userID, ok = g.ActiveUser(w, r)
	if !ok {
		return "", "", false
	}
	_, resolved, ok := g.ExplicitSpace(w, r, userID, explicitSpaceID)
	if !ok {
		return "", "", false
	}
	return userID, resolved, true
}

// MemberOfResourceSpace authorizes a route addressed by a resource ID rather
// than by a space, where the space comes from the record the ID names.
//
// It cannot reuse UserAndPathSpace, which takes the space from the request path;
// here the caller has not said which space it is acting in, and must not be
// allowed to. The refusal is deliberately the same 404 a missing record gets:
// an opaque ID is an identifier and not a credential, and answering 403 would
// make every such route an oracle for whether an ID exists. See
// docs/design/unified-artifacts.md section 6.1.
func (g *Guard) MemberOfResourceSpace(w http.ResponseWriter, r *http.Request, userID, spaceID, notFound string) bool {
	if !httputil.RequireStore(w, g.Spaces, "spaces not configured") {
		return false
	}
	role, ok := g.spaceRole(w, r, userID, spaceID)
	if !ok {
		return false
	}
	if role != "" {
		return true
	}
	g.denied(r, userID, spaceID, DeniedRouteName(r))
	httputil.WriteJSONError(w, http.StatusNotFound, notFound)
	return false
}

// SpaceRole reports the caller's role in the space, or "" when they are not a
// member. It is for a route that has already established membership and needs
// to know how much the member may do.
func (g *Guard) SpaceRole(w http.ResponseWriter, r *http.Request, userID, spaceID string) (string, bool) {
	return g.spaceRole(w, r, userID, spaceID)
}

// spaceRole reads membership from the space's side rather than the caller's.
// ListSpacesByUser would answer membership but not role, and the role is what
// decides whether a member may delete someone else's work.
func (g *Guard) spaceRole(w http.ResponseWriter, r *http.Request, userID, spaceID string) (string, bool) {
	members, err := g.Spaces.ListSpaceMembers(r.Context(), spaceID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_resource_space", "user_id", userID)
		return "", false
	}
	// "" here means the caller is not a member; the bool reports only that the
	// roster read succeeded.
	return corespace.EffectiveRoleOf(members, userID), true
}

// SystemAdmin authorizes a deployment-scoped route.
//
// Deliberately a sibling of SpaceAction rather than a branch inside it. A system
// grant is not an argument to a space check: an administrator reaching a space's
// issues, artifacts, or traces passes the same membership test as anyone else.
// Merging the two would make that boundary depend on nobody ever passing the
// grant down -- see docs/design/system-administration.md section 5.2, and the
// test that fails if this changes.
func (g *Guard) SystemAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	// Authentication first, then the store. The order is the opposite of
	// UserAndStore on purpose: checking the store first would answer an
	// anonymous caller with "system administration not configured", which tells
	// them something about the deployment before they have proved anything
	// about themselves.
	userID, ok := g.ActiveUser(w, r)
	if !ok {
		return "", false
	}
	if !httputil.RequireStore(w, g.Grants, "system administration not configured") {
		return "", false
	}
	roles, err := g.Grants.ActiveSystemRoles(r.Context(), userID)
	if err != nil {
		// A store failure denies. An authorization check that fails open on a
		// database error is the one bug here worth being paranoid about.
		httputil.WriteInternalError(w, err, "handler error", "handler", "require_system_admin", "user_id", userID)
		return "", false
	}
	for _, role := range roles {
		if role == coreidentity.SystemRoleAdmin {
			return userID, true
		}
	}
	// Recorded with an empty space, because the route was not space-scoped. It is
	// the same action a refused space request writes: a denial is what shows
	// someone probing at a boundary, and which boundary is in the target.
	g.denied(r, userID, "", DeniedRouteName(r))
	// 403 rather than 404. Hiding the existence of /api/admin is not achievable
	// -- the routes are in an open-source routes.go and in the Portal bundle --
	// and pretending otherwise would cost a correct status code for no secrecy.
	// What must not leak is data, and this response carries none.
	httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
	return "", false
}

func (g *Guard) denied(r *http.Request, userID, spaceID, target string) {
	if g.Audit == nil {
		return
	}
	g.Audit.Denied(r.Context(), userID, spaceID, target)
}

// DeniedRouteName names the refused route for the audit trail.
//
// The registered pattern rather than the request path: the path carries ids a
// caller chose, and the trail should record which door was tried, not what the
// caller typed into it.
func DeniedRouteName(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.Method + " " + r.URL.Path
}

// RejectDisabled writes the refusal for a disabled account and reports whether
// the caller may continue. The login paths use it so the rule is stated once.
func (g *Guard) RejectDisabled(w http.ResponseWriter, user *coreidentity.User) bool {
	if user == nil || !user.Disabled() {
		return true
	}
	httputil.WriteJSONError(w, http.StatusForbidden, DisabledMessage)
	return false
}

var _ = context.Background
