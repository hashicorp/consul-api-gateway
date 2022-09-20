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
	Marshal() ([]byte, error)
}

type marshaledRoute struct {
	Type  ResolvedRouteType
	Route json.RawMessage
}

// TODO Is this the best location for this func?
func UnmarshalRoute(b []byte) (ResolvedRoute, error) {
	r := &marshaledRoute{}
	if err := json.Unmarshal(b, r); err != nil {
		return nil, err
	}
	switch r.Type {
	case ResolvedHTTPRouteType:
		route := HTTPRoute{}
		if err := json.Unmarshal(r.Route, &route); err != nil {
			return nil, err
		}
		return route, nil
	case ResolvedTCPRouteType:
		route := TCPRoute{}
		if err := json.Unmarshal(r.Route, &route); err != nil {
			return nil, err
		}
		return route, nil
	}
	return nil, errors.New("unsupported route type")
}

// TODO Is this the best location for this func?
func MarshalRoute(route ResolvedRoute) ([]byte, error) {
	data, err := route.Marshal()
	if err != nil {
		return nil, err
	}

	r := &marshaledRoute{
		Type:  route.GetType(),
		Route: data,
	}

	return json.Marshal(r)
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
	ID        GatewayID
	Meta      map[string]string
	Listeners []ResolvedListener
}
