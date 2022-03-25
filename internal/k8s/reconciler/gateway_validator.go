package reconciler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type GatewayValidator struct {
	client gatewayclient.Client
}

func NewGatewayValidator(client gatewayclient.Client) *GatewayValidator {
	return &GatewayValidator{
		client: client,
	}
}

func (g *GatewayValidator) Validate(ctx context.Context, wrappedGateway *K8sGateway) (*GatewayState, error) {
	gateway := wrappedGateway.Gateway

	state := InitialGatewayState(gateway)

	if len(gateway.Spec.Addresses) != 0 {
		state.Status.Ready.AddressNotAssigned = errors.New("gateway does not support requesting addresses")
	}
	g.validateListenerConflicts(state, gateway)
	if err := g.validatePods(ctx, state, gateway); err != nil {
		return nil, err
	}
	service := wrappedGateway.serviceBuilder.Build()
	if err := g.validateGatewayIP(ctx, state, gateway, service); err != nil {
		return nil, err
	}

	listenersReady := true
	listenersInvalid := false
	for i, listenerState := range state.Listeners {
		listener := gateway.Spec.Listeners[i]
		g.validateUnsupported(listenerState, gateway)
		g.validateProtocols(listenerState, listener)

		if err := g.validateTLS(ctx, listenerState, gateway, listener); err != nil {
			return nil, err
		}

		if listenerState.Status.Ready.Invalid == nil && !listenerState.Status.Valid() {
			// set the listener as invalid if any other statuses are not valid
			listenerState.Status.Ready.Invalid = errors.New("listener is in an invalid state")
		}
		if listenerState.Status.Ready.Pending != nil {
			listenersReady = false
		}
		if listenerState.Status.Ready.Invalid != nil {
			listenersInvalid = true
		}
	}

	if listenersInvalid {
		state.Status.Ready.ListenersNotValid = errors.New("gateway listeners not valid")
	} else if !state.PodReady || !state.ServiceReady || !listenersReady {
		state.Status.Ready.ListenersNotReady = errors.New("gateway listeners not ready")
	}

	return state, nil
}

type mergedListener struct {
	port      gw.PortNumber
	listeners []int
	protocols map[string]struct{}
	hostnames map[string]struct{}
}

func mergeListenersByPort(g *gw.Gateway) map[gw.PortNumber]mergedListener {
	mergedListeners := make(map[gw.PortNumber]mergedListener)
	for i, listener := range g.Spec.Listeners {
		merged, found := mergedListeners[listener.Port]
		if !found {
			merged = mergedListener{
				port:      listener.Port,
				protocols: make(map[string]struct{}),
				hostnames: make(map[string]struct{}),
			}
		}
		merged.listeners = append(merged.listeners, i)
		merged.protocols[string(listener.Protocol)] = struct{}{}
		if listener.Hostname != nil {
			merged.hostnames[string(*listener.Hostname)] = struct{}{}
		}
		mergedListeners[listener.Port] = merged
	}
	return mergedListeners
}

func (g *GatewayValidator) validateListenerConflicts(state *GatewayState, gateway *gw.Gateway) {
	for _, merged := range mergeListenersByPort(gateway) {
		if len(merged.protocols) > 1 {
			conflict := fmt.Errorf("listeners have conflicting protocols for port: %s", setToCSV(merged.protocols))
			for _, index := range merged.listeners {
				listenerState := state.Listeners[index]
				listenerState.Status.Conflicted.ProtocolConflict = conflict
			}
		}
		if len(merged.hostnames) > 1 {
			conflict := fmt.Errorf("listeners have conflicting hostnames for port: %s", setToCSV(merged.protocols))
			for _, index := range merged.listeners {
				listenerState := state.Listeners[index]
				listenerState.Status.Conflicted.HostnameConflict = conflict
			}
		}
	}
}

func (g *GatewayValidator) validatePods(ctx context.Context, state *GatewayState, gateway *gw.Gateway) error {
	pod, err := g.client.PodWithLabels(ctx, utils.LabelsForGateway(gateway))
	if err != nil {
		return err
	}

	g.validatePodConditions(state, pod)

	return nil
}

func (g *GatewayValidator) validatePodConditions(state *GatewayState, pod *corev1.Pod) {
	if pod == nil {
		state.Status.Scheduled.NotReconciled = errors.New("pod not found")
		return
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		g.validatePodStatusPending(state, pod)
	case corev1.PodRunning:
		g.validatePodStatusRunning(state, pod)
	case corev1.PodSucceeded:
		// this should never happen, occurs when the pod terminates
		// with a 0 status code, consider this a failed deployment
		fallthrough
	case corev1.PodFailed:
		// we have a failed deployment, set the status accordingly
		// for now we just consider the pods unschedulable.
		state.Status.Scheduled.PodFailed = errors.New("pod not running")
	default: // Unknown pod status
		// we don't have a known pod status, just consider this unreconciled
		state.Status.Scheduled.Unknown = errors.New("pod status unknown")
	}
}

func (g *GatewayValidator) validatePodStatusPending(state *GatewayState, pod *corev1.Pod) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse &&
			strings.Contains(condition.Reason, "Unschedulable") {
			state.Status.Scheduled.NoResources = errors.New(condition.Message)
			return
		}
	}
	// if no conditions exist, or we haven't found a specific above condition, just default
	// to not reconciled
	state.Status.Scheduled.NotReconciled = errors.New("pod conditions not found")
}

func (g *GatewayValidator) validatePodStatusRunning(state *GatewayState, pod *corev1.Pod) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			state.PodReady = true
			return
		}
	}
}

// validateGatewayIP ensures that the appropriate IP addresses are assigned to the
// Gateway.
func (g *GatewayValidator) validateGatewayIP(ctx context.Context, state *GatewayState, gateway *gw.Gateway, service *corev1.Service) error {
	if service == nil {
		return g.assignGatewayIPFromPod(ctx, state, gateway)
	}

	switch service.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		return g.assignGatewayIPFromServiceIngress(ctx, state, service)
	case corev1.ServiceTypeClusterIP:
		return g.assignGatewayIPFromService(ctx, state, service)
	case corev1.ServiceTypeNodePort:
		/* For serviceType: NodePort, there isn't a consistent way to guarantee access to the
		 * service from outside the k8s cluster. For now, we're putting the IP address of the
		 * nodes that the gateway pods are running on.
		 * The practitioner will have to understand that they may need to port forward into the
		 * cluster (in the case of Kind) or open firewall rules (in the case of GKE) in order to
		 * access the gateway from outside the cluster.
		 */
		return g.assignGatewayIPFromPodHost(ctx, state, gateway)
	default:
		return fmt.Errorf("unsupported service type: %s", service.Spec.Type)
	}
}

// assignGatewayIPFromServiceIngress retrieves the external load balancer
// ingress IP for the Service and assigns it to the Gateway
func (g *GatewayValidator) assignGatewayIPFromServiceIngress(ctx context.Context, state *GatewayState, service *corev1.Service) error {
	updated, err := g.client.GetService(ctx, types.NamespacedName{Namespace: service.Namespace, Name: service.Name})
	if err != nil {
		return err
	}

	if updated == nil {
		state.Status.Scheduled.NotReconciled = errors.New("service not found")
		return nil
	}

	for _, ingress := range updated.Status.LoadBalancer.Ingress {
		state.ServiceReady = true
		state.Addresses = append(state.Addresses, ingress.IP)
	}

	return nil
}

// assignGatewayIPFromService retrieves the internal cluster IP for the
// Service and assigns it to the Gateway
func (g *GatewayValidator) assignGatewayIPFromService(ctx context.Context, state *GatewayState, service *corev1.Service) error {
	updated, err := g.client.GetService(ctx, types.NamespacedName{Namespace: service.Namespace, Name: service.Name})
	if err != nil {
		return err
	}

	if updated == nil {
		state.Status.Scheduled.NotReconciled = errors.New("service not found")
		return nil
	}

	if updated.Spec.ClusterIP != "" {
		state.ServiceReady = true
		state.Addresses = append(state.Addresses, updated.Spec.ClusterIP)
	}

	return nil
}

// assignGatewayIPFromPod retrieves the internal IP for the Pod and assigns
// it to the Gateway.
func (g *GatewayValidator) assignGatewayIPFromPod(ctx context.Context, state *GatewayState, gateway *gw.Gateway) error {
	pod, err := g.client.PodWithLabels(ctx, utils.LabelsForGateway(gateway))
	if err != nil {
		return err
	}

	if pod == nil {
		state.Status.Scheduled.NotReconciled = errors.New("pod not found")
		return nil
	}

	if pod.Status.PodIP != "" {
		state.ServiceReady = true
		state.Addresses = append(state.Addresses, pod.Status.PodIP)
	}

	return nil
}

// assignGatewayIPFromPodHost retrieves the (potentially) externally accessible
// IP address for the host that the Pod is running on and assigns it to the Gateway.
// This IP address is not always externally accessible and may require additional
// work by the practitioner such as port-forwarding or opening firewall rules to make
// it externally accessible.
func (g *GatewayValidator) assignGatewayIPFromPodHost(ctx context.Context, state *GatewayState, gateway *gw.Gateway) error {
	pod, err := g.client.PodWithLabels(ctx, utils.LabelsForGateway(gateway))
	if err != nil {
		return err
	}

	if pod == nil {
		state.Status.Scheduled.NotReconciled = errors.New("pod not found")
		return nil
	}

	if pod.Status.HostIP != "" {
		state.ServiceReady = true
		state.Addresses = append(state.Addresses, pod.Status.HostIP)
	}

	return nil
}

func (g *GatewayValidator) validateUnsupported(state *ListenerState, gateway *gw.Gateway) {
	// seems weird that we're looking at gateway fields for listener status
	// but that's the weirdness of the spec
	if len(gateway.Spec.Addresses) > 0 {
		// we dnn't support address binding
		state.Status.Detached.UnsupportedAddress = errors.New("specified addresses are not supported")
	}
}

func (g *GatewayValidator) validateProtocols(state *ListenerState, listener gw.Listener) {
	supportedKinds := supportedKindsFor(listener.Protocol)
	if len(supportedKinds) == 0 {
		state.Status.Detached.UnsupportedProtocol = fmt.Errorf("unsupported protocol: %s", listener.Protocol)
	}
	if listener.AllowedRoutes != nil {
		remainderKinds := kindsNotInSet(listener.AllowedRoutes.Kinds, supportedKinds)
		if len(remainderKinds) != 0 {
			state.Status.ResolvedRefs.InvalidRouteKinds = fmt.Errorf("listener has unsupported kinds: %v", remainderKinds)
		}
	}
}

func (g *GatewayValidator) validateTLS(ctx context.Context, state *ListenerState, gateway *gw.Gateway, listener gw.Listener) error {
	_, tlsRequired := utils.ProtocolToConsul(listener.Protocol)
	if listener.TLS == nil {
		// TODO: should this struct field be "Required" instead of "Enabled"?
		if tlsRequired {
			// we are using a protocol that requires TLS but has no TLS
			// configured
			state.Status.Ready.Invalid = errors.New("tls configuration required for the given protocol")
		}
		return nil
	}

	if listener.TLS.Mode != nil && *listener.TLS.Mode == gw.TLSModePassthrough {
		state.Status.Ready.Invalid = errors.New("tls passthrough not supported")
		return nil
	}

	if len(listener.TLS.CertificateRefs) == 0 {
		state.Status.ResolvedRefs.InvalidCertificateRef = errors.New("certificate reference must be set")
		return nil
	}

	// we only support a single certificate for now
	ref := *listener.TLS.CertificateRefs[0]
	resource, err := resolveCertificateReference(ctx, g.client, gateway, ref)
	if err != nil {
		var certificateErr CertificateResolutionError
		if !errors.As(err, &certificateErr) {
			return err
		}
		state.Status.ResolvedRefs.InvalidCertificateRef = certificateErr
	} else {
		state.TLS.Certificates = []string{resource}
	}

	if listener.TLS.Options != nil {
		tlsMinVersion := listener.TLS.Options[tlsMinVersionAnnotationKey]
		tlsMaxVersion := listener.TLS.Options[tlsMaxVersionAnnotationKey]
		tlsCipherSuitesStr := listener.TLS.Options[tlsCipherSuitesAnnotationKey]

		if tlsMinVersion != "" {
			if _, ok := supportedTlsVersions[string(tlsMinVersion)]; !ok {
				state.Status.Ready.Invalid = errors.New("unrecognized TLS min version")
				return nil
			}

			if tlsCipherSuitesStr != "" {
				if _, ok := tlsVersionsWithConfigurableCipherSuites[string(tlsMinVersion)]; !ok {
					state.Status.Ready.Invalid = errors.New("configuring TLS cipher suites is only supported for TLS 1.2 and earlier")
					return nil
				}
			}

			state.TLS.MinVersion = string(tlsMinVersion)
		}

		if tlsMaxVersion != "" {
			if _, ok := supportedTlsVersions[string(tlsMaxVersion)]; !ok {
				state.Status.Ready.Invalid = errors.New("unrecognized TLS max version")
				return nil
			}

			state.TLS.MaxVersion = string(tlsMaxVersion)
		}

		if tlsCipherSuitesStr != "" {
			// split comma delimited string into string array and trim whitespace
			tlsCipherSuitesUntrimmed := strings.Split(string(tlsCipherSuitesStr), ",")
			tlsCipherSuites := tlsCipherSuitesUntrimmed[:0]
			for _, c := range tlsCipherSuitesUntrimmed {
				tlsCipherSuites = append(tlsCipherSuites, strings.TrimSpace(c))
			}

			// validate each cipher suite in array
			for _, c := range tlsCipherSuites {
				if ok := common.SupportedTLSCipherSuite(c); !ok {
					state.Status.Ready.Invalid = fmt.Errorf("unrecognized or unsupported TLS cipher suite: %s", c)
					return nil
				}
			}

			// set cipher suites on listener TLS params
			state.TLS.CipherSuites = tlsCipherSuites
		}
	}

	return nil
}

func resolveCertificateReference(ctx context.Context, client gatewayclient.Client, gateway *gw.Gateway, ref gw.SecretObjectReference) (string, error) {
	group := corev1.GroupName
	kind := "Secret"
	namespace := gateway.Namespace

	if ref.Group != nil {
		group = string(*ref.Group)
	}
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	switch {
	case kind == "Secret" && group == corev1.GroupName:
		cert, err := client.GetSecret(ctx, types.NamespacedName{Name: string(ref.Name), Namespace: namespace})
		if err != nil {
			return "", fmt.Errorf("error fetching secret: %w", err)
		}
		if cert == nil {
			return "", NewCertificateResolutionErrorNotFound("certificate not found")
		}
		return utils.NewK8sSecret(namespace, string(ref.Name)).String(), nil
	// add more supported types here
	default:
		return "", NewCertificateResolutionErrorUnsupported(fmt.Sprintf("unsupported certificate type - group: %s, kind: %s", group, kind))
	}
}
