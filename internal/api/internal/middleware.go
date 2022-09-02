package internal

import "net/http"

func (s *Server) gatewayTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fill in implementation here

		next.ServeHTTP(w, r)
	})
}
