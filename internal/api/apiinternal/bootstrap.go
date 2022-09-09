package apiinternal

import "net/http"

func (s *Server) Bootstrap(w http.ResponseWriter, r *http.Request) {
	// do the actual bootstrap retrieval here
	send(w, http.StatusOK, &BootstrapConfiguration{
		Name:    "stub-deployment",
		SdsPort: s.bootstrap.SdsPort,
		Consul:  s.bootstrap.Consul,
	})
}
