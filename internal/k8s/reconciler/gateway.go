package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	ErrInvalidGatewayListener    = errors.New("invalid gateway listener")
	ErrTLSPassthroughUnsupported = errors.New("tls passthrough unsupported")
	ErrInvalidTLSConfiguration   = errors.New("invalid tls configuration")
	ErrInvalidTLSCertReference   = errors.New("invalid tls certificate reference")
	ErrCannotBindListener        = errors.New("cannot bind listener")
)

const (
	defaultListenerName = "default"

	ConditionReasonUnableToBind  = "UnableToBindGateway"
	ConditionReasonRouteAdmitted = "RouteAdmitted"
)

// BoundGateway abstracts the logic for tracking
// what has been synced to Consul as well as
// acting as an entrypoint for binding a Route
// to a particular gateway listener.
type BoundGateway struct {
	logger         hclog.Logger
	client         gatewayclient.Client
	consul         *api.Client
	controllerName string

	namespacedName types.NamespacedName
	gateway        *gw.Gateway
	listeners      map[gw.SectionName]*BoundListener

	routers   *consul.ConfigEntryIndex
	splitters *consul.ConfigEntryIndex
	defaults  *consul.ConfigEntryIndex

	mutex sync.RWMutex
}

type BoundGatewayConfig struct {
	Logger         hclog.Logger
	Consul         *api.Client
	Client         gatewayclient.Client
	ControllerName string
}

// NewBoundGateway creates a bound gateway
func NewBoundGateway(gateway *gw.Gateway, config BoundGatewayConfig) *BoundGateway {
	gatewayLogger := config.Logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace)
	namespacedName := utils.NamespacedName(gateway)
	listeners := make(map[gw.SectionName]*BoundListener)
	for _, listener := range gateway.Spec.Listeners {
		listeners[listener.Name] = NewBoundListener(gateway, listener, BoundListenerConfig{
			Logger: gatewayLogger,
			Client: config.Client,
		})
	}

	return &BoundGateway{
		logger:         gatewayLogger,
		client:         config.Client,
		consul:         config.Consul,
		controllerName: config.ControllerName,
		namespacedName: namespacedName,
		gateway:        gateway,
		listeners:      listeners,
		routers:        consul.NewConfigEntryIndex(api.ServiceRouter),
		splitters:      consul.NewConfigEntryIndex(api.ServiceSplitter),
		defaults:       consul.NewConfigEntryIndex(api.ServiceDefaults),
	}
}

func (g *BoundGateway) ResolveListenerReferences(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	var result error
	for _, listener := range g.listeners {
		if err := listener.ResolveCertificates(ctx); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if result != nil {
		return result
	}
	return nil
}

// Merge merges consul tracking information from the supplied gateway.
func (g *BoundGateway) Merge(from *BoundGateway) {
	if from != nil {
		g.mutex.Lock()
		defer g.mutex.Unlock()

		from.mutex.RLock()
		defer from.mutex.RUnlock()

		g.defaults = from.defaults
		g.routers = from.routers
		g.splitters = from.splitters
	}
}

// Bind binds a route to a gateway's listeners if it
// is associated with the gateway
func (g *BoundGateway) Bind(route *K8sRoute) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	statusUpdates := []gw.RouteParentStatus{}
	for _, ref := range route.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := referencesGateway(route.GetNamespace(), ref); isGateway {
			if g.namespacedName == namespacedName {
				bindStatus := g.bindRoute(ref, route)
				statusUpdates = append(statusUpdates, bindStatus)
			} else {
				g.logger.Trace("route does not match gateway", "route", route.GetName(), "wanted-name", namespacedName.Name, "wanted-namespace", namespacedName.Namespace)
			}
		} else {
			g.logger.Trace("route is not a gateway route", "route", route.GetName())
		}
	}

	route.SetAdmittedStatus(statusUpdates...)
	return nil
}

// Remove removes a route from the gateway's listeners if
// it is bound to a listener
func (g *BoundGateway) Remove(namespacedName types.NamespacedName) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, listener := range g.listeners {
		listener.RemoveRoute(namespacedName)
	}
}

func (g *BoundGateway) Sync(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, listener := range g.listeners {
		if listener.ShouldSync() {
			if err := g.sync(ctx); err != nil {
				return err
			}
			break
		}
	}

	for _, listener := range g.listeners {
		listener.SetSynced()
	}

	return nil
}

func (g *BoundGateway) setConfigEntries(ctx context.Context, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, _, err := g.consul.ConfigEntries().Set(entry, options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (g *BoundGateway) deleteConfigEntries(ctx context.Context, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, err := g.consul.ConfigEntries().Delete(entry.GetKind(), entry.GetName(), options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (g *BoundGateway) sync(ctx context.Context) error {
	if g.logger.IsTrace() {
		started := time.Now()
		g.logger.Trace("started reconciliation", "time", started)
		defer g.logger.Trace("reconciliation finished", "time", time.Now(), "spent", time.Since(started))
	}

	ingress := &api.IngressGatewayConfigEntry{
		Kind: api.IngressGateway,
		Name: g.gateway.Name,
		// TODO: namespaces
		Meta: map[string]string{
			"managed_by":                               "consul-api-gateway",
			"consul-api-gateway/k8s/Gateway.Name":      g.gateway.Name,
			"consul-api-gateway/k8s/Gateway.Namespace": g.gateway.Namespace,
		},
	}

	computedRouters := consul.NewConfigEntryIndex(api.ServiceRouter)
	computedSplitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	computedDefaults := consul.NewConfigEntryIndex(api.ServiceDefaults)

	for _, l := range g.listeners {
		listener, routers, splitters, defaults := l.DiscoveryChain()
		if len(listener.Services) > 0 {
			// Consul requires that we have something to route to
			computedRouters.Merge(routers)
			computedSplitters.Merge(splitters)
			computedDefaults.Merge(defaults)
			ingress.Listeners = append(ingress.Listeners, listener)
		} else {
			g.logger.Debug("listener has no services", "name", l.name)
		}
	}

	// Since we can't make multiple config entry changes in a single transaction we must
	// perform the operations in a set that is least likely to induce downtime.
	// First the new service-defaults, routers and splitters should be set
	// Second the ingress gateway
	// Third the removal of any service-defaults, routers or splitters that no longer exist

	addedRouters := computedRouters.ToArray()
	addedDefaults := computedDefaults.ToArray()
	addedSplitters := computedSplitters.ToArray()
	removedRouters := computedRouters.Difference(g.routers).ToArray()
	removedSplitters := computedSplitters.Difference(g.splitters).ToArray()
	removedDefaults := computedDefaults.Difference(g.defaults).ToArray()

	if g.logger.IsTrace() {
		removed, err := json.MarshalIndent(append(append(removedRouters, removedSplitters...), removedDefaults...), "", "  ")
		if err == nil {
			g.logger.Trace("removing", "items", string(removed))
		}
		added, err := json.MarshalIndent(append(append(addedRouters, addedSplitters...), addedDefaults...), "", "  ")
		if err == nil {
			g.logger.Trace("adding", "items", string(added))
		}
	}

	// defaults need to go first, otherwise the routers are always configured to use tcp
	if err := g.setConfigEntries(ctx, addedDefaults...); err != nil {
		return fmt.Errorf("error adding service defaults config entries: %w", err)
	}
	if err := g.setConfigEntries(ctx, addedRouters...); err != nil {
		return fmt.Errorf("error adding service router config entries: %w", err)
	}
	if err := g.setConfigEntries(ctx, addedSplitters...); err != nil {
		return fmt.Errorf("error adding service splitter config entries: %w", err)
	}

	if err := g.setConfigEntries(ctx, ingress); err != nil {
		return fmt.Errorf("error adding ingress config entry: %w", err)
	}

	if err := g.deleteConfigEntries(ctx, removedRouters...); err != nil {
		return fmt.Errorf("error removing service router config entries: %w", err)
	}
	if err := g.deleteConfigEntries(ctx, removedSplitters...); err != nil {
		return fmt.Errorf("error removing service splitter config entries: %w", err)
	}
	if err := g.deleteConfigEntries(ctx, removedDefaults...); err != nil {
		return fmt.Errorf("error removing service defaults config entries: %w", err)
	}

	g.routers = computedRouters
	g.splitters = computedSplitters
	g.defaults = computedDefaults

	return nil
}

func (g *BoundGateway) Equals(other *gw.Gateway) bool {
	if g == nil {
		return false
	}
	return reflect.DeepEqual(g.gateway.Spec, other.Spec)
}

// bindRoute constructs a gateway binding result
// for the given gateway and route, and returns a route
// status based on the result
func (g *BoundGateway) bindRoute(ref gw.ParentRef, route *K8sRoute) gw.RouteParentStatus {
	condition := metav1.Condition{
		Type:               string(gw.ConditionRouteAdmitted),
		ObservedGeneration: route.GetGeneration(),
		LastTransitionTime: metav1.Now(),
	}
	err := g.tryBind(ref, route)
	if err != nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = ConditionReasonUnableToBind
		condition.Message = err.Error()
	} else {
		condition.Status = metav1.ConditionTrue
		condition.Reason = ConditionReasonRouteAdmitted
		condition.Message = "Route allowed"
	}

	return gw.RouteParentStatus{
		ParentRef:  ref,
		Controller: gw.GatewayController(g.controllerName),
		Conditions: []metav1.Condition{condition},
	}
}

// tryBind returns whether a route can bind
// to a gateway, if the route can bind to a listener
// on the gateway the return value is nil, if not,
// an error specifying why the route cannot bind
// is returned.
func (g *BoundGateway) tryBind(ref gw.ParentRef, route *K8sRoute) error {
	bound := false

	var result error
	for _, l := range g.listeners {
		didBind, err := l.TryBind(ref, route)
		if err != nil {
			result = multierror.Append(result, err)
		}

		if didBind {
			bound = true
		}
	}
	// no listeners are bound, so we return an error
	if !bound {
		result = multierror.Append(result, fmt.Errorf("no listeners bound: %w", ErrCannotBindListener))
	}

	if result != nil {
		return result
	}

	return nil
}
