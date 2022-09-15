package v1

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

const (
	AllNamespaces    = "*"
	defaultNamespace = "default"
)

func (s *Server) ListGatewaysInNamespace(w http.ResponseWriter, r *http.Request, namespace string) {
	// do the actual gateway listing here
	stored, err := s.store.ListGateways(r.Context())
	if err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}
	gateways := []Gateway{}
	for _, s := range stored {
		gateways = append(gateways, *s.(*StatefulGateway).Gateway)
	}

	send(w, http.StatusOK, &GatewayPage{
		Gateways: gateways,
	})
}

func (s *Server) ListGateways(w http.ResponseWriter, r *http.Request, params ListGatewaysParams) {
	namespaces := defaultNamespace
	if params.Namespaces != nil {
		namespaces = *params.Namespaces
	}
	s.ListGatewaysInNamespace(w, r, namespaces)
}

func (s *Server) CreateGateway(w http.ResponseWriter, r *http.Request) {
	gateway := &Gateway{}
	if err := json.NewDecoder(r.Body).Decode(gateway); err != nil {
		sendError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.validator.ValidateGateway(r.Context(), gateway); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Info("adding gateway", "gateway", gateway)
	if err := s.store.UpsertGateway(r.Context(), NewStatefulGateway(gateway), nil); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	send(w, http.StatusCreated, gateway)
}

func (s *Server) GetGatewayInNamespace(w http.ResponseWriter, r *http.Request, namespace, name string) {
	// do the actual gateway retrieval here
	gateway, err := s.store.GetGateway(r.Context(), core.GatewayID{ConsulNamespace: namespace, Service: name})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			sendError(w, http.StatusNotFound, "not found")
			return
		}
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	send(w, http.StatusOK, gateway.(*StatefulGateway).Gateway)
}

func (s *Server) GetGateway(w http.ResponseWriter, r *http.Request, name string) {
	s.GetGatewayInNamespace(w, r, defaultNamespace, name)
}

func (s *Server) DeleteGatewayInNamespace(w http.ResponseWriter, r *http.Request, namespace, name string) {
	s.logger.Info("deleting gateway", "namespace", namespace, "name", name)

	if err := s.store.DeleteGateway(r.Context(), core.GatewayID{ConsulNamespace: namespace, Service: name}); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	sendEmpty(w, http.StatusAccepted)
}

func (s *Server) DeleteGateway(w http.ResponseWriter, r *http.Request, name string) {
	s.DeleteGatewayInNamespace(w, r, defaultNamespace, name)
}
