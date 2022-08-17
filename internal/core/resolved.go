package core

type ResolvedService struct {
	ConsulNamespace string
	Service         string
}

type ResolvedRouteType int

const (
	ResolvedHTTPRouteType ResolvedRouteType = iota
	ResolvedTCPRouteType
)

type ResolvedRoute interface {
	GetType() ResolvedRouteType
	GetMeta() map[string]string
	GetName() string
	GetNamespace() string
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
