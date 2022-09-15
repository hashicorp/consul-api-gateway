package v1

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/hashicorp/consul-api-gateway/internal/store"
)

type Marshaler struct{}

type wrappedRoute struct {
	RouteType string          `json:"type"`
	Route     json.RawMessage `json:"route"`
}

func (m *Marshaler) UnmarshalRoute(data []byte) (store.Route, error) {
	wrapped := &wrappedRoute{}
	if err := json.Unmarshal(data, wrapped); err != nil {
		return nil, err
	}
	switch wrapped.RouteType {
	case "http":
		route := &HTTPRoute{}
		if err := json.Unmarshal(wrapped.Route, route); err != nil {
			return nil, err
		}
		return route, nil
	case "tcp":
		route := &TCPRoute{}
		if err := json.Unmarshal(wrapped.Route, route); err != nil {
			return nil, err
		}
		return route, nil
	default:
		return nil, errors.New("invalid route type")
	}
}

func (m *Marshaler) MarshalRoute(iroute store.Route) ([]byte, error) {
	switch route := iroute.(type) {
	case *HTTPRoute:
		d, err := json.Marshal(route)
		if err != nil {
			return nil, err
		}
		return json.Marshal(&wrappedRoute{
			RouteType: "http",
			Route:     d,
		})
	case *TCPRoute:
		d, err := json.Marshal(route)
		if err != nil {
			return nil, err
		}
		return json.Marshal(&wrappedRoute{
			RouteType: "tcp",
			Route:     d,
		})
	default:
		return nil, errors.New("invalid route type")
	}
}

func (m *Marshaler) UnmarshalGateway(data []byte) (store.Gateway, error) {
	log.Printf("unmarshaling data: %s", string(data))
	gateway := &StatefulGateway{}
	if err := json.Unmarshal(data, gateway); err != nil {
		return nil, err
	}
	return gateway.init(), nil
}

func (m *Marshaler) MarshalGateway(gateway store.Gateway) ([]byte, error) {
	return json.Marshal(gateway.(*StatefulGateway))
}
