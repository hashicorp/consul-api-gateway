package v1

import (
	"encoding/json"
	"net/http"
)

func (s *Server) ListNamespacedHTTPRoutes(w http.ResponseWriter, r *http.Request, namespace string) {
	// do the actual route listing here
	sendEmpty(w, http.StatusNotImplemented)
}

func (s *Server) ListHTTPRoutes(w http.ResponseWriter, r *http.Request, params ListHTTPRoutesParams) {
	namespaces := defaultNamespace
	if params.Namespaces != nil {
		namespaces = *params.Namespaces
	}
	s.ListNamespacedHTTPRoutes(w, r, namespaces)
}

func (s *Server) CreateHTTPRoute(w http.ResponseWriter, r *http.Request) {
	route := &HTTPRoute{}
	if err := json.NewDecoder(r.Body).Decode(route); err != nil {
		sendError(w, http.StatusBadRequest, "invalid json")
		return
	}

	s.logger.Info("adding http route", "route", route)
	// do the actual route persistence here

	send(w, http.StatusCreated, route)
}

func (s *Server) GetNamespacedHTTPRoute(w http.ResponseWriter, r *http.Request, namespace, name string) {
	// do the actual gateway retrieval here
	sendEmpty(w, http.StatusNotImplemented)
}

func (s *Server) GetHTTPRoute(w http.ResponseWriter, r *http.Request, name string) {
	s.GetNamespacedHTTPRoute(w, r, defaultNamespace, name)
}

func (s *Server) DeleteNamespacedHTTPRoute(w http.ResponseWriter, r *http.Request, namespace, name string) {
	s.logger.Info("deleting http route", "namespace", namespace, "name", name)
	// do the actual route deletion here

	sendEmpty(w, http.StatusAccepted)
}

func (s *Server) DeleteHTTPRoute(w http.ResponseWriter, r *http.Request, name string) {
	s.DeleteNamespacedHTTPRoute(w, r, defaultNamespace, name)
}
