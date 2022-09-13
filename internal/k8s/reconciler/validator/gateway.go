package validator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	rcommon "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/common"
	rerrors "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/errors"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
)

// GatewayValidator is responsible for taking a provided v1beta1.Gateway and
// deriving a state.GatewayState from it. Ultimately, this GatewayState is what
// makes up the Status on the Kubernetes Gateway resource
// and stores information about currently bound Routes.
type GatewayValidator struct {
	client gatewayclient.Client
}

func NewGatewayValidator(client gatewayclient.Client) *GatewayValidator {
	return &GatewayValidator{
		client: client,
	}
}

func (g *GatewayValidator) Validate(ctx context.Context, gateway *gwv1beta1.Gateway, service *core.Service) (*state.GatewayState, error) {
	state := state.InitialGatewayState(gateway)

	g.validateListenerConflicts(state, gateway)

	if err := g.validatePods(ctx, state, gateway); err != nil {
		return nil, err
	}

	if err := g.validateGatewayIP(ctx, state, gateway, service); err != nil {
		return nil, err
	}

	if len(gateway.Spec.Addresses) != 0 {
		state.Status.Ready.AddressNotAssigned = errors.New("gateway does not support requesting addresses")
	}

	if err := g.validateListeners(ctx, state, gateway); err != nil {
		return nil, err
	}

	return state, nil
}

func (g *GatewayValidator) validateListeners(ctx context.Context, state *state.GatewayState, gateway *gwv1beta1.Gateway) error {
	listenersInvalid, listenersReady := false, true

	for i, listenerState := range state.Listeners {
		listener := gateway.Spec.Listeners[i]
		g.validateUnsupported(listenerState, gateway)
		g.validateProtocols(listenerState, listener)

		if err := g.validateTLS(ctx, listenerState, gateway, listener); err != nil {
			return err
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

	return nil
}

type mergedListener struct {
	port      gwv1beta1.PortNumber
	listeners []int
	protocols map[string]struct{}
	hostnames map[string]struct{}
}

func mergeListenersByPort(g *gwv1beta1.Gateway) map[gwv1beta1.PortNumber]mergedListener {
	mergedListeners := make(map[gwv1beta1.PortNumber]mergedListener)
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

func (g *GatewayValidator) validateListenerConflicts(state *state.GatewayState, gateway *gwv1beta1.Gateway) {
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

func (g *GatewayValidator) validatePods(ctx context.Context, state *state.GatewayState, gateway *gwv1beta1.Gateway) error {
	pods, err := g.client.PodsWithLabels(ctx, utils.LabelsForGateway(gateway))
	if err != nil {
		return err
	}

	for _, pod := range pods {
		g.validatePodConditions(state, &pod)
	}

	return nil
}

func (g *GatewayValidator) validatePodConditions(state *state.GatewayState, pod *core.Pod) {
	if pod == nil {
		state.Status.Scheduled.NotReconciled = errors.New("pod not found")
		return
	}

	switch pod.Status.Phase {
	case core.PodPending:
		g.validatePodStatusPending(state, pod)
	case core.PodRunning:
		g.validatePodStatusRunning(state, pod)
	case core.PodSucceeded:
		// this should never happen, occurs when the pod terminates
		// with a 0 status code, consider this a failed deployment
		fallthrough
	case core.PodFailed:
		// we have a failed deployment, set the status accordingly
		// for now we just consider the pods unschedulable.
		state.Status.Scheduled.PodFailed = errors.New("pod not running")
	default: // Unknown pod status
		// we don't have a known pod status, just consider this unreconciled
		state.Status.Scheduled.Unknown = errors.New("pod status unknown")
	}
}

func (g *GatewayValidator) validatePodStatusPending(state *state.GatewayState, pod *core.Pod) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == core.PodScheduled && condition.Status == core.ConditionFalse &&
			strings.Contains(condition.Reason, "Unschedulable") {
			state.Status.Scheduled.NoResources = errors.New(condition.Message)
			return
		}
	}
	// if no conditions exist, or we haven't found a specific above condition, just default
	// to not reconciled
	state.Status.Scheduled.NotReconciled = errors.New("pod conditions not found")
}

func (g *GatewayValidator) validatePodStatusRunning(state *state.GatewayState, pod *core.Pod) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == core.PodReady && condition.Status == core.ConditionTrue {
			state.PodReady = true
			return
		}
	}
}

// validateGatewayIP ensures that the appropriate IP addresses are assigned to the
// Gateway.
func (g *GatewayValidator) validateGatewayIP(ctx context.Context, state *state.GatewayState, gateway *gwv1beta1.Gateway, service *core.Service) error {
	if service == nil {
		return g.assignGatewayIPFromPods(ctx, state, gateway)
	}

	switch service.Spec.Type {
	case core.ServiceTypeLoadBalancer:
		return g.assignGatewayIPFromServiceIngress(ctx, state, service)
	case core.ServiceTypeClusterIP:
		return g.assignGatewayIPFromService(ctx, state, service)
	case core.ServiceTypeNodePort:
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
func (g *GatewayValidator) assignGatewayIPFromServiceIngress(ctx context.Context, state *state.GatewayState, service *core.Service) error {
	updated, err := g.client.GetService(ctx, types.NamespacedName{Namespace: service.Namespace, Name: service.Name})
	if err != nil {
		return err
	}

	if updated == nil {
		state.Status.Scheduled.NotReconciled = errors.New("service not found")
		return nil
	}

	for _, ingress := range updated.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			state.ServiceReady = true
			state.Addresses = append(state.Addresses, ingress.IP)
		}
		if ingress.Hostname != "" {
			state.ServiceReady = true
			state.Addresses = append(state.Addresses, ingress.Hostname)
		}
	}

	return nil
}

// assignGatewayIPFromService retrieves the internal cluster IP for the
// Service and assigns it to the Gateway
func (g *GatewayValidator) assignGatewayIPFromService(ctx context.Context, state *state.GatewayState, service *core.Service) error {
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
func (g *GatewayValidator) assignGatewayIPFromPods(ctx context.Context, state *state.GatewayState, gateway *gwv1beta1.Gateway) error {
	pods, err := g.client.PodsWithLabels(ctx, utils.LabelsForGateway(gateway))
	if err != nil {
		return err
	}

	if len(pods) == 0 {
		state.Status.Scheduled.NotReconciled = errors.New("pods not found")
		return nil
	}

	for _, pod := range pods {
		if pod.Status.PodIP != "" {
			state.ServiceReady = true
			if !slices.Contains(state.Addresses, pod.Status.PodIP) {
				state.Addresses = append(state.Addresses, pod.Status.PodIP)
			}
		}
	}

	return nil
}

// assignGatewayIPFromPodHost retrieves the (potentially) externally accessible
// IP address for the host that the Pod is running on and assigns it to the Gateway.
// This IP address is not always externally accessible and may require additional
// work by the practitioner such as port-forwarding or opening firewall rules to make
// it externally accessible.
func (g *GatewayValidator) assignGatewayIPFromPodHost(ctx context.Context, state *state.GatewayState, gateway *gwv1beta1.Gateway) error {
	pods, err := g.client.PodsWithLabels(ctx, utils.LabelsForGateway(gateway))
	if err != nil {
		return err
	}

	if len(pods) == 0 {
		state.Status.Scheduled.NotReconciled = errors.New("pods not found")
		return nil
	}

	for _, pod := range pods {
		if pod.Status.HostIP != "" {
			state.ServiceReady = true

			if !slices.Contains(state.Addresses, pod.Status.HostIP) {
				state.Addresses = append(state.Addresses, pod.Status.HostIP)
			}
		}
	}

	return nil
}

func (g *GatewayValidator) validateUnsupported(state *state.ListenerState, gateway *gwv1beta1.Gateway) {
	// seems weird that we're looking at gateway fields for listener status
	// but that's the weirdness of the spec
	if len(gateway.Spec.Addresses) > 0 {
		// we dnn't support address binding
		state.Status.Detached.UnsupportedAddress = errors.New("specified addresses are not supported")
	}
}

func (g *GatewayValidator) validateProtocols(state *state.ListenerState, listener gwv1beta1.Listener) {
	supportedKinds := rcommon.SupportedKindsFor(listener.Protocol)
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

func (g *GatewayValidator) validateTLS(ctx context.Context, state *state.ListenerState, gateway *gwv1beta1.Gateway, listener gwv1beta1.Listener) error {
	if listener.TLS == nil {
		_, tlsRequired := utils.ProtocolToConsul(listener.Protocol)
		if tlsRequired {
			// we are using a protocol that requires TLS but has no TLS configured
			state.Status.Ready.Invalid = errors.New("tls configuration required for the given protocol")
		}
		return nil
	}

	if listener.TLS.Mode != nil && *listener.TLS.Mode == gwv1beta1.TLSModePassthrough {
		state.Status.Ready.Invalid = errors.New("tls passthrough not supported")
		return nil
	}

	if len(listener.TLS.CertificateRefs) == 0 {
		state.Status.ResolvedRefs.InvalidCertificateRef = errors.New("certificate reference must be set")
		return nil
	}

	// we only support a single certificate for now
	ref := listener.TLS.CertificateRefs[0]

	// require ReferenceGrant for cross-namespace certificateRef
	allowed, err := gatewayAllowedForSecretRef(ctx, gateway, ref, g.client)
	if err != nil {
		return err
	} else if !allowed {
		nsName := getNamespacedName(ref.Name, ref.Namespace, gateway.Namespace)
		//g.logger.Warn("Cross-namespace listener certificate not allowed without matching ReferenceGrant", "refName", nsName.Name, "refNamespace", nsName.Namespace)
		state.Status.ResolvedRefs.InvalidCertificateRef = rerrors.NewCertificateResolutionErrorNotPermitted(
			fmt.Sprintf("Cross-namespace listener certificate not allowed without matching ReferenceGrant for Secret %q", nsName))
		return nil
	}

	resource, err := resolveCertificateReference(ctx, g.client, gateway, ref)
	if err != nil {
		var certificateErr rerrors.CertificateResolutionError
		if !errors.As(err, &certificateErr) {
			return err
		}
		state.Status.ResolvedRefs.InvalidCertificateRef = certificateErr
		return nil
	}

	state.TLS.Certificates = []string{resource}

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

func resolveCertificateReference(ctx context.Context, client gatewayclient.Client, gateway *gwv1beta1.Gateway, ref gwv1beta1.SecretObjectReference) (string, error) {
	group := core.GroupName
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
	case kind == "Secret" && group == core.GroupName:
		cert, err := client.GetSecret(ctx, types.NamespacedName{Name: string(ref.Name), Namespace: namespace})
		if err != nil {
			return "", fmt.Errorf("error fetching secret: %w", err)
		}
		if cert == nil {
			return "", rerrors.NewCertificateResolutionErrorNotFound("certificate not found")
		}
		return utils.NewK8sSecret(namespace, string(ref.Name)).String(), nil
	// add more supported types here
	default:
		return "", rerrors.NewCertificateResolutionErrorUnsupported(fmt.Sprintf("unsupported certificate type - group: %s, kind: %s", group, kind))
	}
}

func setToCSV(set map[string]struct{}) string {
	values := []string{}
	for value := range set {
		values = append(values, value)
	}
	return strings.Join(values, ", ")
}

func kindsNotInSet(set, parent []gwv1beta1.RouteGroupKind) []gwv1beta1.RouteGroupKind {
	kinds := []gwv1beta1.RouteGroupKind{}
	for _, kind := range set {
		if !isKindInSet(kind, parent) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func isKindInSet(value gwv1beta1.RouteGroupKind, set []gwv1beta1.RouteGroupKind) bool {
	for _, kind := range set {
		groupsMatch := false
		if value.Group == nil && kind.Group == nil {
			groupsMatch = true
		} else if value.Group != nil && kind.Group != nil && *value.Group == *kind.Group {
			groupsMatch = true
		}
		if groupsMatch && value.Kind == kind.Kind {
			return true
		}
	}
	return false
}

// gatewayAllowedForSecretRef determines whether the gateway is allowed
// for the secret either by being in the same namespace or by having
// an applicable ReferenceGrant in the same namespace as the secret.
func gatewayAllowedForSecretRef(ctx context.Context, gateway *gwv1beta1.Gateway, secretRef gwv1beta1.SecretObjectReference, c gatewayclient.Client) (bool, error) {
	fromNS := gateway.GetNamespace()
	fromGK := metav1.GroupKind{
		Group: gateway.GroupVersionKind().Group,
		Kind:  gateway.GroupVersionKind().Kind,
	}

	toName := string(secretRef.Name)
	toNS := ""
	if secretRef.Namespace != nil {
		toNS = string(*secretRef.Namespace)
	}

	// Kind should default to Secret if not set
	// https://github.com/kubernetes-sigs/gateway-api/blob/ef773194892636ea8ecbb2b294daf771d4dd5009/apis/v1alpha2/object_reference_types.go#L59
	toGK := metav1.GroupKind{Kind: "Secret"}
	if secretRef.Group != nil {
		toGK.Group = string(*secretRef.Group)
	}
	if secretRef.Kind != nil {
		toGK.Kind = string(*secretRef.Kind)
	}

	return referenceAllowed(ctx, fromGK, fromNS, toGK, toNS, toName, c)
}
