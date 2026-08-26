package httpapi

import "net/http"

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-ID") == "" {
			r.Header.Set("X-Request-ID", r.Method+":"+r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
