package v1

import (
	"encoding/json"
	"net/http"
)

func (s *Server) ListHTTPRoutesInNamespace(w http.ResponseWriter, r *http.Request, namespace string) {
	stored, err := s.store.ListRoutes(r.Context())
	if err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}
	routes := []HTTPRoute{}
	for _, s := range stored {
		switch route := s.(type) {
		case *HTTPRoute:
			routes = append(routes, *route)
		}
	}

	send(w, http.StatusOK, &HTTPRoutePage{
		Routes: routes,
	})
}

func (s *Server) ListHTTPRoutes(w http.ResponseWriter, r *http.Request, params ListHTTPRoutesParams) {
	namespaces := defaultNamespace
	if params.Namespaces != nil {
		namespaces = *params.Namespaces
	}
	s.ListHTTPRoutesInNamespace(w, r, namespaces)
}

func (s *Server) CreateHTTPRoute(w http.ResponseWriter, r *http.Request) {
	route := &HTTPRoute{}
	if err := json.NewDecoder(r.Body).Decode(route); err != nil {
		sendError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.validator.ValidateHTTPRoute(r.Context(), route); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("adding http route", "route", route)

	if err := s.store.UpsertRoute(r.Context(), route, nil); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	send(w, http.StatusCreated, route)
}

func (s *Server) GetHTTPRouteInNamespace(w http.ResponseWriter, r *http.Request, namespace, name string) {
	// do the actual gateway retrieval here
	sendEmpty(w, http.StatusNotImplemented)
}

func (s *Server) GetHTTPRoute(w http.ResponseWriter, r *http.Request, name string) {
	s.GetHTTPRouteInNamespace(w, r, defaultNamespace, name)
}

func (s *Server) DeleteHTTPRouteInNamespace(w http.ResponseWriter, r *http.Request, namespace, name string) {
	s.logger.Info("deleting http route", "namespace", namespace, "name", name)

	if err := s.store.DeleteRoute(r.Context(), "httpRoute/"+name); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	sendEmpty(w, http.StatusAccepted)
}

func (s *Server) DeleteHTTPRoute(w http.ResponseWriter, r *http.Request, name string) {
	s.DeleteHTTPRouteInNamespace(w, r, defaultNamespace, name)
}
