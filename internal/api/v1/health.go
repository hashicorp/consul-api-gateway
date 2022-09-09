package v1

import (
	"net/http"

	consulapi "github.com/hashicorp/consul/api"
)

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	options := &consulapi.QueryOptions{
		Namespace: s.namespace,
	}
	services, _, err := s.consulClient.Catalog().Service(s.name, "", options.WithContext(r.Context()))
	if err != nil {
		s.logger.Error("retrieving registered gateway controllers", "error", err)
		sendError(w, http.StatusInternalServerError, "unable to retrieve registered gateway controllers")
	}

	controllerStatuses := []ControllerHealth{}

	for _, service := range services {
		controllerStatuses = append(controllerStatuses, ControllerHealth{
			Name:   service.ServiceName,
			Id:     service.ServiceID, // instance ID
			Status: service.Checks.AggregatedStatus(),
		})
	}

	// retrieve gateways and grab health status of each deployment
	gatewayStatuses := []GatewayHealth{}

	send(w, http.StatusOK, &HealthStatus{
		Controllers: controllerStatuses,
		Gateways:    gatewayStatuses,
	})
}
