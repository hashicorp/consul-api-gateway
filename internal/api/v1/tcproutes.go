package v1

import (
	"encoding/json"
	"net/http"
)

func (s *Server) ListTCPRoutesInNamespace(w http.ResponseWriter, r *http.Request, namespace string) {
	// do the actual route listing here
	sendEmpty(w, http.StatusNotImplemented)
}

func (s *Server) ListTCPRoutes(w http.ResponseWriter, r *http.Request, params ListTCPRoutesParams) {
	namespaces := defaultNamespace
	if params.Namespaces != nil {
		namespaces = *params.Namespaces
	}
	s.ListTCPRoutesInNamespace(w, r, namespaces)
}

func (s *Server) CreateTCPRoute(w http.ResponseWriter, r *http.Request) {
	route := &TCPRoute{}
	if err := json.NewDecoder(r.Body).Decode(route); err != nil {
		sendError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.validator.ValidateTCPRoute(r.Context(), route); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("adding tcp route", "route", route)
	if err := s.store.UpsertRoute(r.Context(), route, nil); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	send(w, http.StatusCreated, route)
}

func (s *Server) GetTCPRouteInNamespace(w http.ResponseWriter, r *http.Request, namespace string, name string) {
	// do the actual gateway retrieval here
	sendEmpty(w, http.StatusNotImplemented)
}

func (s *Server) GetTCPRoute(w http.ResponseWriter, r *http.Request, name string) {
	s.GetTCPRouteInNamespace(w, r, defaultNamespace, name)
}

func (s *Server) DeleteTCPRouteInNamespace(w http.ResponseWriter, r *http.Request, namespace string, name string) {
	s.logger.Info("deleting tcp route", "namespace", namespace, "name", name)
	// do the actual route deletion here

	sendEmpty(w, http.StatusAccepted)
}

func (s *Server) DeleteTCPRoute(w http.ResponseWriter, r *http.Request, name string) {
	s.DeleteTCPRouteInNamespace(w, r, defaultNamespace, name)
}
