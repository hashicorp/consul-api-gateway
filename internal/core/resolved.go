package core

import (
	"encoding/json"
	"errors"
)

type ResolvedService struct {
	ConsulNamespace string
	Service         string
}

type ResolvedRouteType string

const (
	ResolvedHTTPRouteType ResolvedRouteType = "HTTPRoute"
	ResolvedTCPRouteType  ResolvedRouteType = "TCPRoute"
)

type ResolvedRoute interface {
	GetType() ResolvedRouteType
	GetMeta() map[string]string
	GetName() string
	GetNamespace() string
}

// resolvedRouteWrapper serves as a wrapper for a serialized ResolvedRoute.
// We need this wrapper to indicate the type of route serialized in the Data
// field so that it can be deserialized to the correct type.
type resolvedRouteWrapper struct {
	Type ResolvedRouteType
	Data json.RawMessage
}

// UnmarshalResolvedRoute takes a serialized resolvedRouteWrapper and
// deserializes it into the appropriate ResolvedRoute implementation.
// It first partially deserializes to the wrapper in order to get the type
// and then deserializes the Data field into the target HTTPRoute or TCPRoute.
func UnmarshalResolvedRoute(b []byte) (ResolvedRoute, error) {
	// Partially deserialize to determine what the target type is
	wrappedRoute := &resolvedRouteWrapper{}
	if err := json.Unmarshal(b, wrappedRoute); err != nil {
		return nil, err
	}

	// Based on the target type, deserialize into the correct ResolvedRoute impl
	switch wrappedRoute.Type {
	case ResolvedHTTPRouteType:
		route := HTTPRoute{}
		if err := json.Unmarshal(wrappedRoute.Data, &route); err != nil {
			return nil, err
		}
		return route, nil
	case ResolvedTCPRouteType:
		route := TCPRoute{}
		if err := json.Unmarshal(wrappedRoute.Data, &route); err != nil {
			return nil, err
		}
		return route, nil
	default:
		return nil, errors.New("unsupported route type")
	}
}

// MarshalResolvedRoute takes a ResolvedRoute implementation, serializes it, and
// then wraps it in a resolvedRouteWrapper so that we can determine which type
// of xRoute to return on deserialization.
func MarshalResolvedRoute(route ResolvedRoute) ([]byte, error) {
	data, err := json.Marshal(route)
	if err != nil {
		return nil, err
	}

	wrappedRoute := &resolvedRouteWrapper{
		Type: route.GetType(),
		Data: data,
	}

	return json.Marshal(wrappedRoute)
}

type TLSParams struct {
	Enabled      bool
	MinVersion   string
	MaxVersion   string
	CipherSuites []string
	Certificates []string
}

type ResolvedListener struct {
	Name     string
	Hostname string
	Port     int
	Protocol string
	TLS      TLSParams
	Routes   []ResolvedRoute
}

type GatewayID struct {
	ConsulNamespace string
	Service         string
}

type ResolvedGateway struct {
	ID             GatewayID
	Meta           map[string]string
	Listeners      []ResolvedListener
	MaxConnections *uint32
}
