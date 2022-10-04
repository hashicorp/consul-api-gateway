package apiinternal

import "net/http"

const (
	bootstrapTokenHeader = "X-Gateway-Token"
)

func (s *Server) Bootstrap(w http.ResponseWriter, r *http.Request) {
	// enforce that the bootstrap token header is present
	token := r.Header.Get(bootstrapTokenHeader)

	// check that token is valid
	err := validateBootstrapToken(token)

	if err != nil {
		sendError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// TODO: do the actual bootstrap retrieval here
	send(w, http.StatusOK, &BootstrapConfiguration{
		Name:    "stub-deployment",
		SdsPort: s.bootstrap.SdsPort,
		Consul:  s.bootstrap.Consul,
	})
}

func validateBootstrapToken(token string) *Error {
	// TODO: lookup token and validate

	return nil
}
