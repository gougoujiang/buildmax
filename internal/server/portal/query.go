package portal

import (
	"net/url"
	"strconv"
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
