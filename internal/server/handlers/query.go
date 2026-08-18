package handlers

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

func parseLimitOffset(q url.Values, limitKey, offsetKey string, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	if l := q.Get(limitKey); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= maxLimit {
			limit = n
		}
	}
	offset = 0
	if o := q.Get(offsetKey); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// pathValueInt reads a positive integer path segment, such as a revision
// number, and answers the request itself when it is missing or not one.
func pathValueInt(w http.ResponseWriter, r *http.Request, key string) (int, bool) {
	raw, ok := pathValueRequired(w, r, key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		httputil.WriteJSONError(w, http.StatusBadRequest, key+" must be a positive integer")
		return 0, false
	}
	return n, true
}
