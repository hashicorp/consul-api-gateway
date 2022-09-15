package v1

import (
	"context"
	"errors"
	"log"

	"github.com/hashicorp/consul-api-gateway/internal/store"
)

type Binder struct{}

func NewBinder() *Binder {
	return &Binder{}
}

// type Binder interface {
// 	Bind(ctx context.Context, gateway Gateway, route Route) (bool, error)
// 	Unbind(ctx context.Context, gateway Gateway, route Route) (bool, error)
// }

func (b *Binder) Bind(_ context.Context, igateway store.Gateway, iroute store.Route) (bool, error) {
	gateway := igateway.(*StatefulGateway)
	switch route := iroute.(type) {
	case *HTTPRoute:
		log.Println("http binding match")
		name, match, err := references(route.Namespace, "http", gateway.Gateway, route.Gateways)
		if match && err == nil {
			gateway.HTTPRoutes[name][route.ID()] = route
			return true, nil
		}
		return false, err
	case *TCPRoute:
		name, match, err := references(route.Namespace, "tcp", gateway.Gateway, route.Gateways)
		if match && err == nil {
			gateway.TCPRoutes[name][route.ID()] = route
			return true, nil
		}
		return false, err
	default:
		return false, errors.New("invalid route type")
	}
}

func (b *Binder) Unbind(_ context.Context, igateway store.Gateway, iroute store.Route) (bool, error) {
	gateway := igateway.(*StatefulGateway)
	switch route := iroute.(type) {
	case *HTTPRoute:
		for _, l := range gateway.Listeners {
			if s, ok := gateway.HTTPRoutes[stringOrEmpty(l.Name)]; ok {
				if _, ok := s[route.ID()]; ok {
					delete(s, route.ID())
					return true, nil
				}
			}
		}
	case *TCPRoute:
		for _, l := range gateway.Listeners {
			if s, ok := gateway.TCPRoutes[stringOrEmpty(l.Name)]; ok {
				if _, ok := s[route.ID()]; ok {
					delete(s, route.ID())
					return true, nil
				}
			}
		}
	default:
		return false, errors.New("invalid route type")
	}
	return false, nil
}

func references(namespace *string, protocol string, gateway *Gateway, gateways []GatewayReference) (string, bool, error) {
	for _, g := range gateways {
		matchNamespace := ""
		if g.Namespace == nil && namespace != nil {
			matchNamespace = *namespace
		}
		gatewayNamespace := ""
		if gateway.Namespace != nil {
			gatewayNamespace = *gateway.Namespace
		}
		if matchNamespace == gatewayNamespace && g.Name == gateway.Name {
			// check listener binding next
			return mustReferenceListeners(protocol, g.Listener, gateway.Listeners)
		}
	}
	return "", false, nil
}

func mustReferenceListeners(protocol string, listener *string, listeners []Listener) (string, bool, error) {
	name := ""
	if listener != nil {
		name = *listener
	}
	for _, l := range listeners {
		listenerName := ""
		if l.Name != nil {
			listenerName = *l.Name
		}
		if name == listenerName {
			// check the protocol, if it doesn't match, then error
			if string(l.Protocol) == protocol {
				return name, true, nil
			}
			return "", false, errors.New("protocol doesn't match listener protocol")
		}
	}
	// no name == listenerName
	return "", false, errors.New("invalid listener name")
}
