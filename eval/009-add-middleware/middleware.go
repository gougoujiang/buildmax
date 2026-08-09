package api

import "net/http"

// RequestID is an HTTP middleware that adds a unique X-Request-ID header to
// every response. Each request must receive a distinct, non-empty identifier.
//
// TODO: implement this function.
//   - Generate a unique string ID per request (e.g. using fmt.Sprintf with an
//     atomic counter, or math/rand, or crypto/rand).
//   - Set it as the "X-Request-ID" response header before calling next.
//   - Call next.ServeHTTP to pass the request down the chain.
func RequestID(next http.Handler) http.Handler {
	return next // placeholder — does not add the header yet
}
