package reconciler

type GatewayState struct {
	Status       GatewayStatus
	PodReady     bool
	ServiceReady bool
	Addresses    []string
}
