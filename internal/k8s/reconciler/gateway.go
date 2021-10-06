package reconciler

import (
	"context"
	"errors"
	"reflect"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	ConditionTypeSynced          = "Synced"
	ConditionReasonSyncFailed    = "SyncFailed"
	ConditionReasonSyncSucceeded = "SyncSucceeded"
)

type K8sGateway struct {
	consulNamespace string
	logger          hclog.Logger
	client          gatewayclient.Client
	gateway         *gw.Gateway
	syncedStatus    *metav1.Condition
	tracker         GatewayStatusTracker
	listeners       map[string]*K8sListener
}

var _ core.StatusTrackingGateway = &K8sGateway{}

type K8sGatewayConfig struct {
	ConsulNamespace string
	Logger          hclog.Logger
	Tracker         GatewayStatusTracker
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
		consulNamespace: config.ConsulNamespace,
		logger:          gatewayLogger,
		client:          config.Client,
		tracker:         config.Tracker,
		gateway:         gateway,
		listeners:       listeners,
	}
}

func (g *K8sGateway) ID() core.GatewayID {
	return core.GatewayID{
		Service:         g.gateway.Name,
		ConsulNamespace: g.consulNamespace,
	}
}

func (g *K8sGateway) Logger() hclog.Logger {
	return g.logger
}

func (g *K8sGateway) Meta() map[string]string {
	return map[string]string{
		"managed_by":                               "consul-api-gateway",
		"consul-api-gateway/k8s/Gateway.Name":      g.gateway.Name,
		"consul-api-gateway/k8s/Gateway.Namespace": g.gateway.Namespace,
	}
}

func (g *K8sGateway) Listeners() []core.Listener {
	listeners := []core.Listener{}

	for _, listener := range g.listeners {
		listeners = append(listeners, listener)
	}

	return listeners
}

func (g *K8sGateway) Compare(other core.Gateway) core.CompareResult {
	if other == nil {
		return core.CompareResultInvalid
	}
	if g == nil {
		return core.CompareResultNotEqual
	}

	if otherGateway, ok := other.(*K8sGateway); ok {
		if g.gateway.Generation > otherGateway.gateway.Generation {
			return core.CompareResultNewer
		}
		if reflect.DeepEqual(g.gateway.Spec, otherGateway.gateway.Spec) {
			return core.CompareResultEqual
		}
		return core.CompareResultNotEqual
	}
	return core.CompareResultInvalid
}

func (g *K8sGateway) ShouldBind(route core.Route) bool {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return false
	}
	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := referencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			if utils.NamespacedName(g.gateway) == namespacedName {
				return true
			}
		}
	}

	return false
}

func (g *K8sGateway) TrackSync(ctx context.Context, sync func() (bool, error)) error {
	namedGateway := utils.NamespacedName(g.gateway)
	pod, err := g.client.PodWithLabels(ctx, utils.LabelsForGateway(g.gateway))
	if err != nil {
		if !errors.Is(err, gatewayclient.ErrPodNotCreated) {
			return err
		}
	}

	conditions := utils.MapGatewayConditionsFromPod(pod)
	var result error

	didSync, err := sync()
	syncStatusUpdated := false
	if err != nil {
		syncStatusUpdated = true
		g.logger.Trace("gateway sync failed, updating sync status")
		g.syncedStatus = &metav1.Condition{
			Type:               ConditionTypeSynced,
			Reason:             ConditionReasonSyncFailed,
			Message:            err.Error(),
			Status:             metav1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
		}
		result = multierror.Append(result, err)
	} else if didSync {
		syncStatusUpdated = true
		g.logger.Trace("synced gateway, updating sync status")
		g.syncedStatus = &metav1.Condition{
			Type:               ConditionTypeSynced,
			Reason:             ConditionReasonSyncSucceeded,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
		}
	}

	if err := g.tracker.UpdateStatus(namedGateway, pod, conditions, syncStatusUpdated, func() error {
		if g.syncedStatus != nil {
			conditions = append(conditions, *g.syncedStatus)
		}
		g.gateway.Status.Conditions = conditions
		return g.client.UpdateStatus(ctx, g.gateway)
	}); err != nil {
		return multierror.Append(result, err)
	}

	if result != nil {
		return result
	}
	return nil
}
