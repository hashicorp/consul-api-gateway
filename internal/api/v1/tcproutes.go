package v1

import (
	"encoding/json"
	"net/http"
)

func (s *Server) ListNamespacedTCPRoutes(w http.ResponseWriter, r *http.Request, namespace string) {
	// do the actual route listing here
	sendEmpty(w, http.StatusNotImplemented)
}

func (s *Server) ListTCPRoutes(w http.ResponseWriter, r *http.Request, params ListTCPRoutesParams) {
	namespaces := defaultNamespace
	if params.Namespaces != nil {
		namespaces = *params.Namespaces
	}
	s.ListNamespacedTCPRoutes(w, r, namespaces)
}

func (s *Server) CreateTCPRoute(w http.ResponseWriter, r *http.Request) {
	route := &TCPRoute{}
	if err := json.NewDecoder(r.Body).Decode(route); err != nil {
		sendError(w, http.StatusBadRequest, "invalid json")
		return
	}

	s.logger.Info("adding http route", "route", route)
	// do the actual route persistence here

	send(w, http.StatusCreated, route)
}

func (s *Server) GetNamespacedTCPRoute(w http.ResponseWriter, r *http.Request, namespace string, name string) {
	// do the actual gateway retrieval here
	sendEmpty(w, http.StatusNotImplemented)
}

func (s *Server) GetTCPRoute(w http.ResponseWriter, r *http.Request, name string) {
	s.GetNamespacedTCPRoute(w, r, defaultNamespace, name)
}

func (s *Server) DeleteNamespacedTCPRoute(w http.ResponseWriter, r *http.Request, namespace string, name string) {
	s.logger.Info("deleting tcp route", "namespace", namespace, "name", name)
	// do the actual route deletion here

	sendEmpty(w, http.StatusAccepted)
}

func (s *Server) DeleteTCPRoute(w http.ResponseWriter, r *http.Request, name string) {
	s.DeleteNamespacedTCPRoute(w, r, defaultNamespace, name)
}
