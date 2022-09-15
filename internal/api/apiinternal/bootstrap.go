package apiinternal

import "net/http"

func (s *Server) Bootstrap(w http.ResponseWriter, r *http.Request) {
	// if r.Header.Get("x-consul-token") != "cab6f60f-0383-4701-a5f2-49f37bc3f7cc" {
	// 	sendError(w, http.StatusUnauthorized, "unauthorized")
	// 	return
	// }
	// do the actual bootstrap retrieval here along with token header enforcement
	send(w, http.StatusOK, &BootstrapConfiguration{
		Name:    "stub-deployment",
		SdsPort: s.bootstrap.SdsPort,
		Consul:  s.bootstrap.Consul,
	})
}
