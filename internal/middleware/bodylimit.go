package middleware

import "net/http"

// BodyLimit wraps the request body with MaxBytesReader so that handlers
// cannot be coerced into allocating an unbounded buffer. Applied to every
// request that carries a body.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
