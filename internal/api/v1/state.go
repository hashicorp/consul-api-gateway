package v1

import (
	"fmt"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/vault"
)

type StatefulGateway struct {
	*Gateway

	TCPRoutes  map[string]map[string]*TCPRoute
	HTTPRoutes map[string]map[string]*HTTPRoute
}

func NewStatefulGateway(gateway *Gateway) *StatefulGateway {
	return (&StatefulGateway{
		Gateway: gateway,
	}).init()
}

func (s *StatefulGateway) init() *StatefulGateway {
	tcp := map[string]map[string]*TCPRoute{}
	http := map[string]map[string]*HTTPRoute{}
	for _, l := range s.Gateway.Listeners {
		switch l.Protocol {
		case ListenerProtocolHttp:
			http[stringOrEmpty(l.Name)] = map[string]*HTTPRoute{}
		case ListenerProtocolTcp:
			tcp[stringOrEmpty(l.Name)] = map[string]*TCPRoute{}
		}
	}
	if s.HTTPRoutes == nil {
		s.HTTPRoutes = http
	}
	if s.TCPRoutes == nil {
		s.TCPRoutes = tcp
	}
	return s
}

func (s *StatefulGateway) ID() core.GatewayID {
	return core.GatewayID{
		ConsulNamespace: s.namespace(),
		Service:         s.Gateway.Name,
	}
}

func (s *StatefulGateway) namespace() string {
	if s.Gateway.Namespace == nil {
		return ""
	}
	return *s.Gateway.Namespace
}

func (s *StatefulGateway) Resolve() core.ResolvedGateway {
	listeners := []core.ResolvedListener{}
	for _, l := range s.Gateway.Listeners {
		listeners = append(listeners, s.resolveListener(l))
	}
	return core.ResolvedGateway{
		ID:        s.ID(),
		Listeners: listeners,
	}
}

func (s *StatefulGateway) resolveListener(listener Listener) core.ResolvedListener {
	name := stringOrEmpty(listener.Name)
	return core.ResolvedListener{
		Name:     name,
		Protocol: string(listener.Protocol),
		Hostname: listener.Hostname,
		Port:     listener.Port,
		TLS:      tlsFor(listener.Tls),
		Routes:   s.routesFor(listener),
	}
}

func (s *StatefulGateway) routesFor(listener Listener) []core.ResolvedRoute {
	switch listener.Protocol {
	case ListenerProtocolHttp:
		routes := []core.ResolvedRoute{}
		for _, r := range s.HTTPRoutes[stringOrEmpty(listener.Name)] {
			routes = append(routes, s.httpRouteToCore(listener.Hostname, stringOrEmpty(s.Gateway.Namespace), r))
		}
		return routes
	case ListenerProtocolTcp:
		routes := []core.ResolvedRoute{}
		for _, r := range s.TCPRoutes[stringOrEmpty(listener.Name)] {
			routes = append(routes, tcpRouteToCore(stringOrEmpty(s.Gateway.Namespace), r))
		}
		return routes
	}
	return nil
}

func (s *StatefulGateway) httpRouteToCore(hostname, namespace string, route *HTTPRoute) core.ResolvedRoute {
	return NewHTTPRouteConverter(HTTPRouteConverterConfig{
		Namespace: namespace,
		Hostname:  hostname,
		Prefix:    fmt.Sprintf("consul-api-gateway_%s_", s.Gateway.Name),
		Meta: map[string]string{
			"external-source": "consul-api-gateway",
		},
		Route: route,
	}).Convert()
}

func tcpRouteToCore(namespace string, route *TCPRoute) core.ResolvedRoute {
	return core.NewTCPRouteBuilder().
		WithName(route.Name).
		WithNamespace(namespace).
		WithService(core.ResolvedService{
			ConsulNamespace: stringOrEmpty(route.Services[0].Namespace),
			Service:         route.Services[0].Name,
		}).
		Build()
}

func tlsFor(config *TLSConfiguration) core.TLSParams {
	if config == nil {
		return core.TLSParams{}
	}
	return core.TLSParams{
		Enabled:      true,
		MinVersion:   stringOrEmpty(config.MinVersion),
		MaxVersion:   stringOrEmpty(config.MaxVersion),
		CipherSuites: config.CipherSuites,
		Certificates: certsToIDs(config.Certificates),
	}
}

func certsToIDs(certs []Certificate) []string {
	ids := []string{}
	for _, c := range certs {
		ids = append(ids, vault.NewKVSecret(c.Vault.Path, c.Vault.ChainField, c.Vault.PrivateKeyField).String())
	}
	return ids
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *StatefulGateway) CanFetchSecrets(secrets []string) (bool, error) {
	return false, nil
}

func (h *HTTPRoute) ID() string {
	return "httpRoute/" + h.Name
}

func (h *TCPRoute) ID() string {
	return "tcpRoute"
}
