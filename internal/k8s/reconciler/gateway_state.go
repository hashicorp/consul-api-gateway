package reconciler

// GatewayState holds ephemeral state for gateways
type GatewayState struct {
	Status       GatewayStatus
	PodReady     bool
	ServiceReady bool
	Addresses    []string
}
