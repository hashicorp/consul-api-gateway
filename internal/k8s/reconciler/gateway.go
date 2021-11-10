package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	corev1 "k8s.io/api/core/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type K8sGateway struct {
	consulNamespace string
	logger          hclog.Logger
	client          gatewayclient.Client
	gateway         *gw.Gateway
	config          apigwv1alpha1.GatewayClassConfig
	sdsConfig       apigwv1alpha1.SDSConfig

	status    GatewayStatus
	podReady  bool
	addresses []string
	listeners map[string]*K8sListener
}

var _ store.StatusTrackingGateway = &K8sGateway{}

type K8sGatewayConfig struct {
	ConsulNamespace string
	SDSConfig       apigwv1alpha1.SDSConfig
	Config          apigwv1alpha1.GatewayClassConfig
	Logger          hclog.Logger
	Client          gatewayclient.Client
}

func NewK8sGateway(gateway *gw.Gateway, config K8sGatewayConfig) *K8sGateway {
	gatewayLogger := config.Logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace)
	listeners := make(map[string]*K8sListener)
	for _, listener := range gateway.Spec.Listeners {
		k8sListener := NewK8sListener(gateway, listener, K8sListenerConfig{
			ConsulNamespace: config.ConsulNamespace,
			Logger:          gatewayLogger,
			Client:          config.Client,
		})
		listeners[k8sListener.ID()] = k8sListener
	}

	return &K8sGateway{
		config:          config.Config,
		sdsConfig:       config.SDSConfig,
		consulNamespace: config.ConsulNamespace,
		logger:          gatewayLogger,
		client:          config.Client,
		gateway:         gateway,
		listeners:       listeners,
	}
}

func (g *K8sGateway) certificates() []string {
	certificates := []string{}
	for _, listener := range g.listeners {
		certificates = append(certificates, listener.Certificates()...)
	}
	return certificates
}

func (g *K8sGateway) Validate(ctx context.Context) error {
	g.status = GatewayStatus{}
	g.validateListenerConflicts()

	if err := g.validatePods(ctx); err != nil {
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

	if pod.Status.PodIP != "" {
		g.addresses = append(g.addresses, pod.Status.PodIP)
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

func (g *K8sGateway) Compare(other store.Gateway) store.CompareResult {
	if other == nil {
		return store.CompareResultInvalid
	}
	if g == nil {
		return store.CompareResultNotEqual
	}

	if otherGateway, ok := other.(*K8sGateway); ok {
		if utils.ResourceVersionGreater(g.gateway.ResourceVersion, otherGateway.gateway.ResourceVersion) {
			return store.CompareResultNewer
		}

		if !g.isEqual(otherGateway) {
			return store.CompareResultNotEqual
		}
		return store.CompareResultEqual
	}
	return store.CompareResultInvalid
}

func (g *K8sGateway) isEqual(other *K8sGateway) bool {
	if !reflect.DeepEqual(g.gateway.Spec, other.gateway.Spec) {
		return false
	}
	if !gatewayStatusEqual(g.gateway.Status, other.gateway.Status) {
		return false
	}

	// check other things that may affect the pending status updates
	if !reflect.DeepEqual(g.certificates(), other.certificates()) {
		return false
	}
	if !conditionEqual(g.status.Scheduled.Condition(g.gateway.Generation), other.status.Scheduled.Condition(g.gateway.Generation)) {
		return false
	}
	if g.podReady != other.podReady {
		return false
	}
	if !reflect.DeepEqual(g.addresses, other.addresses) {
		return false
	}

	return true
}

func (g *K8sGateway) ShouldBind(route store.Route) bool {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return false
	}

	if !k8sRoute.IsValid() {
		g.logger.Trace("route is invalid, should not bind", "route", route.ID())
		return false
	}

	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := utils.ReferencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			if utils.NamespacedName(g.gateway) == namespacedName {
				return true
			}
		}
	}
	g.logger.Trace("route does not reference gateway, should not bind", "route", route.ID())
	return false
}

func (g *K8sGateway) Status() gw.GatewayStatus {
	listenerStatuses := []gw.ListenerStatus{}
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
		g.status.Ready.ListenersNotValid = errors.New("gateway listeners not valid")
	} else if !g.podReady || !listenersReady {
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
	addresses := []gw.GatewayAddress{}
	for _, address := range g.addresses {
		addresses = append(addresses, gw.GatewayAddress{
			Type:  &ipType,
			Value: address,
		})
	}

	// TODO: set addresses based off of pod/service lookup
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
	deployment := g.config.DeploymentFor(g.gateway, g.sdsConfig)
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
	if service := g.config.ServiceFor(g.gateway); service != nil {
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
