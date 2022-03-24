package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/builder"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
)

type K8sGateway struct {
	consulNamespace   string
	logger            hclog.Logger
	client            gatewayclient.Client
	gateway           *gw.Gateway
	config            apigwv1alpha1.GatewayClassConfig
	deploymentBuilder builder.DeploymentBuilder
	serviceBuilder    builder.ServiceBuilder

	status       GatewayStatus
	podReady     bool
	serviceReady bool
	addresses    []string
	listeners    map[string]*K8sListener
}

var _ store.StatusTrackingGateway = &K8sGateway{}

func (g *K8sGateway) Validate(ctx context.Context) error {
	g.status = GatewayStatus{}
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
	port      gw.PortNumber
	listeners []*K8sListener
	protocols map[string]struct{}
	hostnames map[string]struct{}
}

func (g *K8sGateway) mergeListenersByPort() map[gw.PortNumber]mergedListener {
	mergedListeners := make(map[gw.PortNumber]mergedListener)
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
				listener.ListenerState.Status.Conflicted.ProtocolConflict = conflict
			}
		}
		if len(merged.hostnames) > 1 {
			conflict := fmt.Errorf("listeners have conflicting hostnames for port: %s", setToCSV(merged.protocols))
			for _, listener := range merged.listeners {
				listener.ListenerState.Status.Conflicted.HostnameConflict = conflict
			}
		}
	}
}

// validateGatewayIP ensures that the appropriate IP addresses are assigned to the
// Gateway.
func (g *K8sGateway) validateGatewayIP(ctx context.Context) error {
	service := g.serviceBuilder.Build()
	if service == nil {
		return g.assignGatewayIPFromPod(ctx)
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
		g.status.Scheduled.NotReconciled = errors.New("service not found")
		return nil
	}

	for _, ingress := range updated.Status.LoadBalancer.Ingress {
		g.serviceReady = true
		g.addresses = append(g.addresses, ingress.IP)
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
		g.status.Scheduled.NotReconciled = errors.New("service not found")
		return nil
	}

	if updated.Spec.ClusterIP != "" {
		g.serviceReady = true
		g.addresses = append(g.addresses, updated.Spec.ClusterIP)
	}

	return nil
}

// assignGatewayIPFromPod retrieves the internal IP for the Pod and assigns
// it to the Gateway.
func (g *K8sGateway) assignGatewayIPFromPod(ctx context.Context) error {
	pod, err := g.client.PodWithLabels(ctx, utils.LabelsForGateway(g.gateway))
	if err != nil {
		return err
	}

	if pod == nil {
		g.status.Scheduled.NotReconciled = errors.New("pod not found")
		return nil
	}

	if pod.Status.PodIP != "" {
		g.serviceReady = true
		g.addresses = append(g.addresses, pod.Status.PodIP)
	}

	return nil
}

// assignGatewayIPFromPodHost retrieves the (potentially) externally accessible
// IP address for the host that the Pod is running on and assigns it to the Gateway.
// This IP address is not always externally accessible and may require additional
// work by the practitioner such as port-forwarding or opening firewall rules to make
// it externally accessible.
func (g *K8sGateway) assignGatewayIPFromPodHost(ctx context.Context) error {
	pod, err := g.client.PodWithLabels(ctx, utils.LabelsForGateway(g.gateway))
	if err != nil {
		return err
	}

	if pod == nil {
		g.status.Scheduled.NotReconciled = errors.New("pod not found")
		return nil
	}

	if pod.Status.HostIP != "" {
		g.serviceReady = true
		g.addresses = append(g.addresses, pod.Status.HostIP)
	}

	return nil
}

func (g *K8sGateway) validatePods(ctx context.Context) error {
	pod, err := g.client.PodWithLabels(ctx, utils.LabelsForGateway(g.gateway))
	if err != nil {
		return err
	}

	g.validatePodConditions(pod)

	return nil
}

func (g *K8sGateway) validatePodConditions(pod *corev1.Pod) {
	if pod == nil {
		g.status.Scheduled.NotReconciled = errors.New("pod not found")
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
		g.status.Scheduled.PodFailed = errors.New("pod not running")
	default: // Unknown pod status
		// we don't have a known pod status, just consider this unreconciled
		g.status.Scheduled.Unknown = errors.New("pod status unknown")
	}
}

func (g *K8sGateway) validatePodStatusPending(pod *corev1.Pod) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse &&
			strings.Contains(condition.Reason, "Unschedulable") {
			g.status.Scheduled.NoResources = errors.New(condition.Message)
			return
		}
	}
	// if no conditions exist, or we haven't found a specific above condition, just default
	// to not reconciled
	g.status.Scheduled.NotReconciled = errors.New("pod conditions not found")
}

func (g *K8sGateway) validatePodStatusRunning(pod *corev1.Pod) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			g.podReady = true
			return
		}
	}
}

func (g *K8sGateway) ID() core.GatewayID {
	return core.GatewayID{
		Service:         g.gateway.Name,
		ConsulNamespace: g.consulNamespace,
	}
}

func (g *K8sGateway) Meta() map[string]string {
	return map[string]string{
		"external-source":                          "consul-api-gateway",
		"consul-api-gateway/k8s/Gateway.Name":      g.gateway.Name,
		"consul-api-gateway/k8s/Gateway.Namespace": g.gateway.Namespace,
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

	return !utils.ResourceVersionGreater(g.gateway.ResourceVersion, otherGateway.gateway.ResourceVersion)
}

func (g *K8sGateway) ShouldBind(route store.Route) bool {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return false
	}

	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := utils.ReferencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			if utils.NamespacedName(g.gateway) == namespacedName {
				return true
			}
		}
	}
	return false
}

func (g *K8sGateway) Status() gw.GatewayStatus {
	listenerStatuses := []gw.ListenerStatus{}
	listenersReady := true
	listenersInvalid := false
	for _, listener := range g.listeners {
		if listener.ListenerState.Status.Ready.Pending != nil {
			listenersReady = false
		}
		if listener.ListenerState.Status.Ready.Invalid != nil {
			listenersInvalid = true
		}
		listenerStatuses = append(listenerStatuses, listener.Status())
	}

	if listenersInvalid {
		g.status.Ready.ListenersNotValid = errors.New("gateway listeners not valid")
	} else if !g.podReady || !g.serviceReady || !listenersReady {
		g.status.Ready.ListenersNotReady = errors.New("gateway listeners not ready")
	} else if len(g.gateway.Spec.Addresses) != 0 {
		g.status.Ready.AddressNotAssigned = errors.New("gateway does not support requesting addresses")
	}
	conditions := g.status.Conditions(g.gateway.Generation)

	// prefer to not update to not mess up timestamps
	if listenerStatusesEqual(listenerStatuses, g.gateway.Status.Listeners) {
		listenerStatuses = g.gateway.Status.Listeners
	}
	if conditionsEqual(conditions, g.gateway.Status.Conditions) {
		conditions = g.gateway.Status.Conditions
	}

	ipType := gw.IPAddressType
	addresses := make([]gw.GatewayAddress, 0, len(g.addresses))
	for _, address := range g.addresses {
		addresses = append(addresses, gw.GatewayAddress{
			Type:  &ipType,
			Value: address,
		})
	}

	return gw.GatewayStatus{
		Addresses:  addresses,
		Conditions: conditions,
		Listeners:  listenerStatuses,
	}
}

func (g *K8sGateway) TrackSync(ctx context.Context, sync func() (bool, error)) error {
	// we've done all but synced our state, so ensure our deployments are up-to-date
	if err := g.ensureDeploymentExists(ctx); err != nil {
		return err
	}

	didSync, err := sync()
	if err != nil {
		g.status.InSync.SyncError = err
	} else if didSync {
		// clear out any old synchronization error statuses
		g.status.InSync = GatewayInSyncStatus{}
	}

	status := g.Status()
	if !gatewayStatusEqual(status, g.gateway.Status) {
		g.gateway.Status = status
		if g.logger.IsTrace() {
			data, err := json.MarshalIndent(status, "", "  ")
			if err == nil {
				g.logger.Trace("setting gateway status", "status", string(data))
			}
		}
		if err := g.client.UpdateStatus(ctx, g.gateway); err != nil {
			// make sure we return an error immediately that's unwrapped
			return err
		}
	}
	return nil
}

func (g *K8sGateway) ensureDeploymentExists(ctx context.Context) error {
	// Create service account for the gateway
	if serviceAccount := g.config.ServiceAccountFor(g.gateway); serviceAccount != nil {
		if err := g.client.EnsureServiceAccount(ctx, g.gateway, serviceAccount); err != nil {
			return err
		}
	}

	deployment := g.deploymentBuilder.Build()
	mutated := deployment.DeepCopy()
	if updated, err := g.client.CreateOrUpdateDeployment(ctx, mutated, func() error {
		mutated = apigwv1alpha1.MergeDeployment(deployment, mutated)
		return g.client.SetControllerOwnership(g.gateway, mutated)
	}); err != nil {
		return fmt.Errorf("failed to create or update gateway deployment: %w", err)
	} else if updated {
		if g.logger.IsTrace() {
			data, err := json.MarshalIndent(mutated, "", "  ")
			if err == nil {
				g.logger.Trace("created or updated gateway deployment", "deployment", string(data))
			}
		}
	}

	// Create service for the gateway
	if service := g.serviceBuilder.Build(); service != nil {
		mutated := service.DeepCopy()
		if updated, err := g.client.CreateOrUpdateService(ctx, mutated, func() error {
			mutated = apigwv1alpha1.MergeService(service, mutated)
			return g.client.SetControllerOwnership(g.gateway, mutated)
		}); err != nil {
			return fmt.Errorf("failed to create or update gateway service: %w", err)
		} else if updated {
			if g.logger.IsTrace() {
				data, err := json.MarshalIndent(mutated, "", "  ")
				if err == nil {
					g.logger.Trace("created or updated gateway service", "service", string(data))
				}
			}
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
