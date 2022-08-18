package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/go-hclog"
	"golang.org/x/exp/slices"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	rstatus "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/status"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

type K8sGateway struct {
	*gwv1beta1.Gateway
	GatewayState *state.GatewayState

	logger   hclog.Logger
	client   gatewayclient.Client
	config   apigwv1alpha1.GatewayClassConfig
	deployer *GatewayDeployer

	listeners []*K8sListener
}

var _ store.StatusTrackingGateway = &K8sGateway{}

type K8sGatewayConfig struct {
	ConsulNamespace string
	ConsulCA        string
	SDSHost         string
	SDSPort         int
	Config          apigwv1alpha1.GatewayClassConfig
	Deployer        *GatewayDeployer
	Logger          hclog.Logger
	Client          gatewayclient.Client
}

func NewK8sGateway(gateway *gwv1beta1.Gateway, config K8sGatewayConfig) *K8sGateway {
	gatewayLogger := config.Logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace)
	listeners := make([]*K8sListener, 0, len(gateway.Spec.Listeners))
	for _, listener := range gateway.Spec.Listeners {
		k8sListener := NewK8sListener(gateway, listener, K8sListenerConfig{
			ConsulNamespace: config.ConsulNamespace,
			Logger:          gatewayLogger,
			Client:          config.Client,
		})
		listeners = append(listeners, k8sListener)
	}

	// TODO
	gwState := state.InitialGatewayState(gateway)
	gwState.ConsulNamespace = config.ConsulNamespace

	return &K8sGateway{
		Gateway:      gateway,
		GatewayState: gwState,
		config:       config.Config,
		deployer:     config.Deployer,
		logger:       gatewayLogger,
		client:       config.Client,
		listeners:    listeners,
	}
}

func (g *K8sGateway) Validate(ctx context.Context) error {
	g.GatewayState.Status = rstatus.GatewayStatus{}
	g.validateListenerConflicts()

	if err := g.validatePods(ctx); err != nil {
		return err
	}

	if err := g.validateGatewayIP(ctx); err != nil {
		return err
	}

	for _, listener := range g.listeners {
		if err := listener.Validate(ctx); err != nil {
			return err
		}
	}

	return nil
}

type mergedListener struct {
	port      gwv1beta1.PortNumber
	listeners []*K8sListener
	protocols map[string]struct{}
	hostnames map[string]struct{}
}

func (g *K8sGateway) mergeListenersByPort() map[gwv1beta1.PortNumber]mergedListener {
	mergedListeners := make(map[gwv1beta1.PortNumber]mergedListener)
	for _, listener := range g.listeners {
		merged, found := mergedListeners[listener.listener.Port]
		if !found {
			merged = mergedListener{
				port:      listener.listener.Port,
				protocols: make(map[string]struct{}),
				hostnames: make(map[string]struct{}),
			}
		}
		merged.listeners = append(merged.listeners, listener)
		merged.protocols[string(listener.listener.Protocol)] = struct{}{}
		if listener.listener.Hostname != nil {
			merged.hostnames[string(*listener.listener.Hostname)] = struct{}{}
		}
		mergedListeners[listener.listener.Port] = merged
	}
	return mergedListeners
}

func (g *K8sGateway) validateListenerConflicts() {
	for _, merged := range g.mergeListenersByPort() {
		if len(merged.protocols) > 1 {
			conflict := fmt.Errorf("listeners have conflicting protocols for port: %s", setToCSV(merged.protocols))
			for _, listener := range merged.listeners {
				listener.status.Conflicted.ProtocolConflict = conflict
			}
		}
		if len(merged.hostnames) > 1 {
			conflict := fmt.Errorf("listeners have conflicting hostnames for port: %s", setToCSV(merged.protocols))
			for _, listener := range merged.listeners {
				listener.status.Conflicted.HostnameConflict = conflict
			}
		}
	}
}

// validateGatewayIP ensures that the appropriate IP addresses are assigned to the
// Gateway.
func (g *K8sGateway) validateGatewayIP(ctx context.Context) error {
	service := g.deployer.Service(g.config, g.Gateway)
	if service == nil {
		return g.assignGatewayIPFromPods(ctx)
	}

	switch service.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		return g.assignGatewayIPFromServiceIngress(ctx, service)
	case corev1.ServiceTypeClusterIP:
		return g.assignGatewayIPFromService(ctx, service)
	case corev1.ServiceTypeNodePort:
		/* For serviceType: NodePort, there isn't a consistent way to guarantee access to the
		 * service from outside the k8s cluster. For now, we're putting the IP address of the
		 * nodes that the gateway pods are running on.
		 * The practitioner will have to understand that they may need to port forward into the
		 * cluster (in the case of Kind) or open firewall rules (in the case of GKE) in order to
		 * access the gateway from outside the cluster.
		 */
		return g.assignGatewayIPFromPodHost(ctx)
	default:
		return fmt.Errorf("unsupported service type: %s", service.Spec.Type)
	}
}

// assignGatewayIPFromServiceIngress retrieves the external load balancer
// ingress IP for the Service and assigns it to the Gateway
func (g *K8sGateway) assignGatewayIPFromServiceIngress(ctx context.Context, service *corev1.Service) error {
	updated, err := g.client.GetService(ctx, types.NamespacedName{Namespace: service.Namespace, Name: service.Name})
	if err != nil {
		return err
	}

	if updated == nil {
		g.GatewayState.Status.Scheduled.NotReconciled = errors.New("service not found")
		return nil
	}

	for _, ingress := range updated.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			g.GatewayState.ServiceReady = true
			g.GatewayState.Addresses = append(g.GatewayState.Addresses, ingress.IP)
		}
		if ingress.Hostname != "" {
			g.GatewayState.ServiceReady = true
			g.GatewayState.Addresses = append(g.GatewayState.Addresses, ingress.Hostname)
		}
	}

	return nil
}

// assignGatewayIPFromService retrieves the internal cluster IP for the
// Service and assigns it to the Gateway
func (g *K8sGateway) assignGatewayIPFromService(ctx context.Context, service *corev1.Service) error {
	updated, err := g.client.GetService(ctx, types.NamespacedName{Namespace: service.Namespace, Name: service.Name})
	if err != nil {
		return err
	}

	if updated == nil {
		g.GatewayState.Status.Scheduled.NotReconciled = errors.New("service not found")
		return nil
	}

	if updated.Spec.ClusterIP != "" {
		g.GatewayState.ServiceReady = true
		g.GatewayState.Addresses = append(g.GatewayState.Addresses, updated.Spec.ClusterIP)
	}

	return nil
}

// assignGatewayIPFromPods retrieves the internal IP for the Pods and assigns
// it to the Gateway.
func (g *K8sGateway) assignGatewayIPFromPods(ctx context.Context) error {
	pods, err := g.client.PodsWithLabels(ctx, utils.LabelsForGateway(g.Gateway))
	if err != nil {
		return err
	}

	if len(pods) == 0 {
		g.GatewayState.Status.Scheduled.NotReconciled = errors.New("pods not found")
		return nil
	}

	for _, pod := range pods {
		if pod.Status.PodIP != "" {
			g.GatewayState.ServiceReady = true
			if !slices.Contains(g.GatewayState.Addresses, pod.Status.PodIP) {
				g.GatewayState.Addresses = append(g.GatewayState.Addresses, pod.Status.PodIP)
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
func (g *K8sGateway) assignGatewayIPFromPodHost(ctx context.Context) error {
	pods, err := g.client.PodsWithLabels(ctx, utils.LabelsForGateway(g.Gateway))
	if err != nil {
		return err
	}

	if len(pods) == 0 {
		g.GatewayState.Status.Scheduled.NotReconciled = errors.New("pods not found")
		return nil
	}

	for _, pod := range pods {
		if pod.Status.HostIP != "" {
			g.GatewayState.ServiceReady = true

			if !slices.Contains(g.GatewayState.Addresses, pod.Status.HostIP) {
				g.GatewayState.Addresses = append(g.GatewayState.Addresses, pod.Status.HostIP)
			}
		}
	}

	return nil
}

func (g *K8sGateway) validatePods(ctx context.Context) error {
	pods, err := g.client.PodsWithLabels(ctx, utils.LabelsForGateway(g.Gateway))
	if err != nil {
		return err
	}

	for _, pod := range pods {
		g.validatePodConditions(&pod)
	}

	return nil
}

func (g *K8sGateway) validatePodConditions(pod *corev1.Pod) {
	if pod == nil {
		g.GatewayState.Status.Scheduled.NotReconciled = errors.New("pod not found")
		return
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		g.validatePodStatusPending(pod)
	case corev1.PodRunning:
		g.validatePodStatusRunning(pod)
	case corev1.PodSucceeded:
		// this should never happen, occurs when the pod terminates
		// with a 0 status code, consider this a failed deployment
		fallthrough
	case corev1.PodFailed:
		// we have a failed deployment, set the status accordingly
		// for now we just consider the pods unschedulable.
		g.GatewayState.Status.Scheduled.PodFailed = errors.New("pod not running")
	default: // Unknown pod status
		// we don't have a known pod status, just consider this unreconciled
		g.GatewayState.Status.Scheduled.Unknown = errors.New("pod status unknown")
	}
}

func (g *K8sGateway) validatePodStatusPending(pod *corev1.Pod) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse &&
			strings.Contains(condition.Reason, "Unschedulable") {
			g.GatewayState.Status.Scheduled.NoResources = errors.New(condition.Message)
			return
		}
	}
	// if no conditions exist, or we haven't found a specific above condition, just default
	// to not reconciled
	g.GatewayState.Status.Scheduled.NotReconciled = errors.New("pod conditions not found")
}

func (g *K8sGateway) validatePodStatusRunning(pod *corev1.Pod) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			g.GatewayState.PodReady = true
			return
		}
	}
}

func (g *K8sGateway) ID() core.GatewayID {
	return core.GatewayID{
		Service:         g.Gateway.Name,
		ConsulNamespace: g.GatewayState.ConsulNamespace,
	}
}

func (g *K8sGateway) Meta() map[string]string {
	return map[string]string{
		"external-source":                          "consul-api-gateway",
		"consul-api-gateway/k8s/Gateway.Name":      g.Gateway.Name,
		"consul-api-gateway/k8s/Gateway.Namespace": g.Gateway.Namespace,
	}
}

func (g *K8sGateway) Listeners() []store.Listener {
	listeners := []store.Listener{}

	for _, listener := range g.listeners {
		listeners = append(listeners, listener)
	}

	return listeners
}

func (g *K8sGateway) ShouldUpdate(other store.Gateway) bool {
	if other == nil {
		return false
	}

	if g == nil {
		return true
	}

	otherGateway, ok := other.(*K8sGateway)
	if !ok {
		return false
	}

	return !utils.ResourceVersionGreater(g.Gateway.ResourceVersion, otherGateway.Gateway.ResourceVersion)
}

func (g *K8sGateway) ShouldBind(route store.Route) bool {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return false
	}

	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := utils.ReferencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			if utils.NamespacedName(g.Gateway) == namespacedName {
				return true
			}
		}
	}
	return false
}

func (g *K8sGateway) Status() gwv1beta1.GatewayStatus {
	listenerStatuses := []gwv1beta1.ListenerStatus{}
	listenersReady := true
	listenersInvalid := false
	for _, listener := range g.listeners {
		if listener.status.Ready.Pending != nil {
			listenersReady = false
		}
		if listener.status.Ready.Invalid != nil {
			listenersInvalid = true
		}
		listenerStatuses = append(listenerStatuses, listener.Status())
	}

	if listenersInvalid {
		g.GatewayState.Status.Ready.ListenersNotValid = errors.New("gateway listeners not valid")
	} else if !g.GatewayState.PodReady || !g.GatewayState.ServiceReady || !listenersReady {
		g.GatewayState.Status.Ready.ListenersNotReady = errors.New("gateway listeners not ready")
	} else if len(g.Gateway.Spec.Addresses) != 0 {
		g.GatewayState.Status.Ready.AddressNotAssigned = errors.New("gateway does not support requesting addresses")
	}
	conditions := g.GatewayState.Status.Conditions(g.Gateway.Generation)

	// prefer to not update to not mess up timestamps
	if listenerStatusesEqual(listenerStatuses, g.Gateway.Status.Listeners) {
		listenerStatuses = g.Gateway.Status.Listeners
	}
	if conditionsEqual(conditions, g.Gateway.Status.Conditions) {
		conditions = g.Gateway.Status.Conditions
	}

	ipType := gwv1beta1.IPAddressType
	addresses := make([]gwv1beta1.GatewayAddress, 0, len(g.GatewayState.Addresses))
	for _, address := range g.GatewayState.Addresses {
		addresses = append(addresses, gwv1beta1.GatewayAddress{
			Type:  &ipType,
			Value: address,
		})
	}

	return gwv1beta1.GatewayStatus{
		Addresses:  addresses,
		Conditions: conditions,
		Listeners:  listenerStatuses,
	}
}

func (g *K8sGateway) TrackSync(ctx context.Context, sync func() (bool, error)) error {
	// we've done all but synced our state, so ensure our deployments are up-to-date
	if err := g.deployer.Deploy(ctx, g.GatewayState.ConsulNamespace, g.config, g.Gateway); err != nil {
		return err
	}

	didSync, err := sync()
	if err != nil {
		g.GatewayState.Status.InSync.SyncError = err
	} else if didSync {
		// clear out any old synchronization error statuses
		g.GatewayState.Status.InSync = rstatus.GatewayInSyncStatus{}
	}

	status := g.Status()
	if !gatewayStatusEqual(status, g.Gateway.Status) {
		g.Gateway.Status = status
		if g.logger.IsTrace() {
			data, err := json.MarshalIndent(status, "", "  ")
			if err == nil {
				g.logger.Trace("setting gateway status", "status", string(data))
			}
		}
		if err := g.client.UpdateStatus(ctx, g.Gateway); err != nil {
			// make sure we return an error immediately that's unwrapped
			return err
		}
	}
	return nil
}

func setToCSV(set map[string]struct{}) string {
	values := []string{}
	for value := range set {
		values = append(values, value)
	}
	return strings.Join(values, ", ")
}
