package k8s

type RouteType string

const (
	RouteTypeHTTPRoute RouteType = "HTTPRoute"
	RouteTypeTCPRoute  RouteType = "TCPRoute"
	RouteTypeTLSRoute  RouteType = "TLSRoute"
	RouteTypeUDPRoute  RouteType = "UDPRoute"
)
