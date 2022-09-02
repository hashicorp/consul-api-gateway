package v1

import (
	"encoding/json"
	"net/http"
)

const (
	defaultNamespace = "default"
)

func (s *Server) ListNamespacedGateways(w http.ResponseWriter, r *http.Request, namespace string) {
	// do the actual gateway listing here
	s.sendError(w, http.StatusNotImplemented, "Not implemented")
}

func (s *Server) ListGateways(w http.ResponseWriter, r *http.Request, params ListGatewaysParams) {
	namespaces := defaultNamespace
	if params.Namespaces != nil {
		namespaces = *params.Namespaces
	}
	s.ListNamespacedGateways(w, r, namespaces)
}

func (s *Server) CreateGateway(w http.ResponseWriter, r *http.Request) {
	gateway := &Gateway{}
	if err := json.NewDecoder(r.Body).Decode(gateway); err != nil {
		sendError(w, http.StatusBadRequest, "invalid json")
		return
	}

	s.logger.Info("adding gateway", "gateway", gateway)
	// do the actual gateway persistence here

	send(w, http.StatusCreated, gateway)
}

func (s *Server) GetNamespacedGateway(w http.ResponseWriter, r *http.Request, namespace, name string) {
	// do the actual gateway retrieval here
	s.sendError(w, http.StatusNotImplemented, "Not implemented")
}

func (s *Server) GetGateway(w http.ResponseWriter, r *http.Request, name string) {
	s.GetNamespacedGateway(w, r, defaultNamespace, name)
}

func (s *Server) DeleteGateway(w http.ResponseWriter, r *http.Request, name string) {
	s.DeleteNamespacedGateway(w, r, defaultNamespace, name)
}

func (s *Server) DeleteNamespacedGateway(w http.ResponseWriter, r *http.Request, namespace, name string) {
	s.logger.Info("deleting gateway", "namespace", namespace, "name", name)
	// do the actual gateway deletion here

	sendEmpty(w, http.StatusAccepted)
}
