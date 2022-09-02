package v1

import (
	"encoding/json"
	"net/http"
)

func (s *Server) ListHTTPRoutes(w http.ResponseWriter, r *http.Request) {
	// do the actual route listing here
	sendEmpty(w, http.StatusNotImplemented)
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

func (s *Server) GetHTTPRoute(w http.ResponseWriter, r *http.Request, namespace string, name string) {
	// do the actual gateway retrieval here
	sendEmpty(w, http.StatusNotImplemented)
}

func (s *Server) DeleteHTTPRoute(w http.ResponseWriter, r *http.Request, namespace string, name string) {
	s.logger.Info("deleting http route", "namespace", namespace, "name", name)
	// do the actual route deletion here

	sendEmpty(w, http.StatusAccepted)
}
