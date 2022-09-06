package internal

import "net/http"

func (s *Server) Bootstrap(w http.ResponseWriter, r *http.Request) {
	// do the actual bootstrap retrieval here
	sendError(w, http.StatusNotImplemented, "Not implemented")
}
