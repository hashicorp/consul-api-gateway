package apiinternal

import "net/http"

func (s *Server) gatewayTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fill in implementation with bootstrap token checking here

		next.ServeHTTP(w, r)
	})
}
