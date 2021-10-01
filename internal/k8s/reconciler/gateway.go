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

// BoundGateway wraps a gateway and its listeners
type BoundGateway struct {
	logger hclog.Logger

	controllerName string
	namespacedName types.NamespacedName
	gateway        *gw.Gateway
	listeners      map[gw.SectionName]*BoundListener

	routers   *consul.ConfigEntryIndex
	splitters *consul.ConfigEntryIndex
	defaults  *consul.ConfigEntryIndex

	mutex sync.RWMutex
}

func NewBoundGateway(ctx context.Context, logger hclog.Logger, controllerName string, client gatewayclient.Client, gateway *gw.Gateway, from *BoundGateway) (*BoundGateway, error) {
	namespacedName := utils.NamespacedName(gateway)

	g := &BoundGateway{
		logger:         logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace),
		controllerName: controllerName,
		namespacedName: namespacedName,
		gateway:        gateway,
		listeners:      make(map[gw.SectionName]*BoundListener),
		routers:        consul.NewConfigEntryIndex(api.ServiceRouter),
		splitters:      consul.NewConfigEntryIndex(api.ServiceSplitter),
		defaults:       consul.NewConfigEntryIndex(api.ServiceDefaults),
	}
	for _, listener := range gateway.Spec.Listeners {
		boundListener, err := NewBoundListener(ctx, client, g.logger, g.gateway, listener)
		if err != nil {
			return nil, err
		}
		g.listeners[listener.Name] = boundListener
	}

	if from != nil {
		from.mutex.RLock()
		defer from.mutex.RUnlock()

		g.defaults = from.defaults
		g.routers = from.routers
		g.splitters = from.splitters
	}
	return g, nil
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
				listeners, bindStatus := bindRoute(g.controllerName, ref, g.gateway, route)
				if len(listeners) == 0 {
					g.logger.Trace("route did not bind", "route", route.GetName(), "status", bindStatus)
				}
				statusUpdates = append(statusUpdates, bindStatus)
				for _, listener := range listeners {
					boundListener, found := g.listeners[listener.Name]
					if !found {
						return ErrInvalidGatewayListener
					}
					boundListener.SetRoute(route)
				}
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

func (g *BoundGateway) Sync(ctx context.Context, consul *api.Client) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, listener := range g.listeners {
		if listener.ShouldSync() {
			if err := g.sync(ctx, consul); err != nil {
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

func setConfigEntries(ctx context.Context, client *api.Client, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, _, err := client.ConfigEntries().Set(entry, options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func deleteConfigEntries(ctx context.Context, client *api.Client, entries ...api.ConfigEntry) error {
	options := &api.WriteOptions{}
	var result error
	for _, entry := range entries {
		if _, err := client.ConfigEntries().Delete(entry.GetKind(), entry.GetName(), options.WithContext(ctx)); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (g *BoundGateway) sync(ctx context.Context, client *api.Client) error {
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
	if err := setConfigEntries(ctx, client, addedDefaults...); err != nil {
		return fmt.Errorf("error adding service defaults config entries: %w", err)
	}
	if err := setConfigEntries(ctx, client, addedRouters...); err != nil {
		return fmt.Errorf("error adding service router config entries: %w", err)
	}
	if err := setConfigEntries(ctx, client, addedSplitters...); err != nil {
		return fmt.Errorf("error adding service splitter config entries: %w", err)
	}

	if err := setConfigEntries(ctx, client, ingress); err != nil {
		return fmt.Errorf("error adding ingress config entry: %w", err)
	}

	if err := deleteConfigEntries(ctx, client, removedRouters...); err != nil {
		return fmt.Errorf("error removing service router config entries: %w", err)
	}
	if err := deleteConfigEntries(ctx, client, removedSplitters...); err != nil {
		return fmt.Errorf("error removing service splitter config entries: %w", err)
	}
	if err := deleteConfigEntries(ctx, client, removedDefaults...); err != nil {
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
func bindRoute(controllerName string, ref gw.ParentRef, gateway *gw.Gateway, route *K8sRoute) ([]gw.Listener, gw.RouteParentStatus) {
	condition := metav1.Condition{
		Type:               string(gw.ConditionRouteAdmitted),
		ObservedGeneration: route.GetGeneration(),
		LastTransitionTime: metav1.Now(),
	}
	listeners, err := routeCanBind(ref, gateway, route)
	if err != nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = ConditionReasonUnableToBind
		condition.Message = err.Error()
	} else {
		condition.Status = metav1.ConditionTrue
		condition.Reason = ConditionReasonRouteAdmitted
		condition.Message = "Route allowed"
	}

	return listeners, gw.RouteParentStatus{
		ParentRef:  ref,
		Controller: gw.GatewayController(controllerName),
		Conditions: []metav1.Condition{condition},
	}
}

// routeCanBind returns whether a route can bind
// to a gateway, if the route can bind to a listener
// on the gateway the return value is nil, if not,
// an error specifying why the route cannot bind
// is returned.
func routeCanBind(ref gw.ParentRef, gateway *gw.Gateway, route *K8sRoute) ([]gw.Listener, error) {
	var boundListeners []gw.Listener
	for _, listener := range gateway.Spec.Listeners {
		// must is only true if there's a ref with a specific listener name
		// meaning if we must attach, but cannot, it's an error
		allowed, must := routeMatchesListener(listener.Name, ref.SectionName)
		if allowed {
			if !routeKindIsAllowedForListener(listener.AllowedRoutes, route) {
				if must {
					return nil, fmt.Errorf("route kind not allowed for listener: %w", ErrCannotBindListener)
				}
				continue
			}
			allowed, err := routeAllowedForListenerNamespaces(gateway.Namespace, listener.AllowedRoutes, route)
			if err != nil {
				return nil, fmt.Errorf("error checking listener namespaces: %w", err)
			}
			if !allowed {
				if must {
					return nil, fmt.Errorf("route not allowed because of listener namespace policy: %w", ErrCannotBindListener)
				}
				continue
			}

			if !route.MatchesHostname(listener.Hostname) {
				if must {
					return nil, fmt.Errorf("route does not match listener hostname: %w", ErrCannotBindListener)
				}
				continue
			}

			boundListeners = append(boundListeners, listener)
		}
	}
	// no listeners are bound, so we return an error
	if len(boundListeners) == 0 {
		return nil, fmt.Errorf("no listeners bound: %w", ErrCannotBindListener)
	}

	return boundListeners, nil
}
