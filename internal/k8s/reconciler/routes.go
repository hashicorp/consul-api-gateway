package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type resolvedReferenceType int

var (
	ErrEmptyPort            = errors.New("port cannot be empty with kubernetes service")
	ErrNotResolved          = errors.New("backend reference not found")
	ErrConsulNotResolved    = errors.New("consul service not found")
	ErrUnsupportedReference = errors.New("unsupported reference type")
)

const (
	// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
	NamespaceNameLabel     = "kubernetes.io/metadata.name"
	MetaKeyKubeServiceName = "k8s-service-name"
	MetaKeyKubeNS          = "k8s-namespace"

	routeReference resolvedReferenceType = iota
	consulServiceReference
)

type consulService struct {
	namespace string
	name      string
}

type resolvedReference struct {
	referenceType resolvedReferenceType
	ref           BackendRef
	consulService *consulService
}

// all kubernetes routes implement the following two interfaces
type Route interface {
	client.Object
	schema.ObjectKind
}

type RouteRule struct {
	httpRule *gw.HTTPRouteRule
}

type BackendRef struct {
	httpRef *gw.HTTPBackendRef
}

type K8sRoute struct {
	Route

	logger          hclog.Logger
	controllerName  string
	needsStatusSync bool
	isResolved      bool
	references      map[RouteRule][]resolvedReference

	// this mutex protects any of the above private fields
	mutex sync.RWMutex
}

func NewK8sRoute(controllerName string, logger hclog.Logger, route Route) *K8sRoute {
	return &K8sRoute{
		Route:           route,
		logger:          logger.Named("route").With("name", route.GetName()),
		controllerName:  controllerName,
		needsStatusSync: true,
	}
}

func (r *K8sRoute) MatchesHostname(hostname *gw.Hostname) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return routeMatchesListenerHostname(hostname, route.Spec.Hostnames)
	default:
		return true
	}
}

func (r *K8sRoute) CommonRouteSpec() gw.CommonRouteSpec {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return route.Spec.CommonRouteSpec
	case *gw.TCPRoute:
		return route.Spec.CommonRouteSpec
	case *gw.UDPRoute:
		return route.Spec.CommonRouteSpec
	case *gw.TLSRoute:
		return route.Spec.CommonRouteSpec
	}
	return gw.CommonRouteSpec{}
}

func (r *K8sRoute) RouteStatus() gw.RouteStatus {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.routeStatus()
}

func (r *K8sRoute) routeStatus() gw.RouteStatus {
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		return route.Status.RouteStatus
	case *gw.TCPRoute:
		return route.Status.RouteStatus
	case *gw.UDPRoute:
		return route.Status.RouteStatus
	case *gw.TLSRoute:
		return route.Status.RouteStatus
	}
	return gw.RouteStatus{}
}

func (r *K8sRoute) SetAdmittedStatus(statuses ...gw.RouteParentStatus) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	updated := setAdmittedStatus(r.routeStatus(), statuses...)

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TCPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.UDPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TLSRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	}
}

func (r *K8sRoute) ClearParentStatus(namespacedName types.NamespacedName) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	updated := clearParentStatus(r.controllerName, r.GetName(), r.routeStatus(), namespacedName)

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TCPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.UDPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TLSRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	}
}

func (r *K8sRoute) setStatus(current, updated gw.RouteStatus) gw.RouteStatus {
	if len(current.Parents) != len(updated.Parents) {
		r.logger.Trace("marking route status as dirty")
		r.needsStatusSync = true
		return updated
	}

	sort.SliceStable(current.Parents, func(i, j int) bool {
		return compareJSON(current.Parents[i]) > compareJSON(current.Parents[j])
	})
	sort.SliceStable(updated.Parents, func(i, j int) bool {
		return compareJSON(updated.Parents[i]) > compareJSON(updated.Parents[j])
	})

	if utils.IsFieldUpdated(current, updated) {
		r.logger.Trace("marking route status as dirty")
		r.needsStatusSync = true
		return updated
	}
	return current
}

func (r *K8sRoute) setResolvedRefsStatus(statuses ...gw.RouteParentStatus) {
	updated := setResolvedRefsStatus(r.routeStatus(), statuses...)

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TCPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.UDPRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	case *gw.TLSRoute:
		route.Status.RouteStatus = r.setStatus(route.Status.RouteStatus, updated)
	}
}

func (r *K8sRoute) ResolveReferences(ctx context.Context, client gatewayclient.Client, consul *api.Client) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.isResolved {
		return nil
	}

	var result error

	resolved := make(map[RouteRule][]resolvedReference)
	var parents []gw.ParentRef
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		parents = route.Spec.ParentRefs
		for _, rule := range route.Spec.Rules {
			routeRule := RouteRule{httpRule: &rule}
			for _, ref := range rule.BackendRefs {
				resolvedRef, err := resolveBackendReference(ctx, client, ref.BackendObjectReference, r, consul)
				if err != nil {
					result = multierror.Append(result, err)
					continue
				}
				if resolvedRef != nil {
					resolvedRef.ref = BackendRef{
						httpRef: &ref,
					}
					resolved[routeRule] = append(resolved[routeRule], *resolvedRef)
				}
			}
		}
	default:
		return nil
	}
	reason := string(gw.ConditionRouteResolvedRefs)
	message := string(gw.ConditionRouteResolvedRefs)
	resolvedStatus := metav1.ConditionTrue
	if result != nil {
		reason = "InvalidRefs"
		message = result.Error()
		resolvedStatus = metav1.ConditionFalse
	}
	conditions := []metav1.Condition{{
		Type:               string(gw.ConditionRouteResolvedRefs),
		Status:             resolvedStatus,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}}

	// this seems odd to set this on the parent ref
	statuses := []gw.RouteParentStatus{}
	for _, ref := range parents {
		statuses = append(statuses, gw.RouteParentStatus{
			ParentRef:  ref,
			Controller: gw.GatewayController(r.controllerName),
			Conditions: conditions,
		})
	}
	r.setResolvedRefsStatus(statuses...)

	if result == nil {
		r.isResolved = true
	}
	r.references = resolved
	return result
}

func (r *K8sRoute) UpdateStatus(ctx context.Context, client gatewayclient.Client) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.needsStatusSync {
		if r.logger.IsTrace() {
			status, err := json.MarshalIndent(r.routeStatus(), "", "  ")
			if err == nil {
				r.logger.Trace("syncing route status", "status", string(status))
			}
		}
		if err := client.UpdateStatus(ctx, r.Route); err != nil {
			return fmt.Errorf("error updating route status: %w", err)
		}
		r.needsStatusSync = false
	}

	return nil
}

func (r *K8sRoute) DiscoveryChain(prefix, hostname string, meta map[string]string) (*api.IngressService, *api.ServiceRouterConfigEntry, *consul.ConfigEntryIndex, *consul.ConfigEntryIndex) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	splitters := consul.NewConfigEntryIndex(api.ServiceSplitter)
	defaults := consul.NewConfigEntryIndex(api.ServiceDefaults)
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		router, splits := httpRouteToServiceDiscoChain(r, prefix, meta)
		serviceDefault := httpServiceDefault(router, meta)
		defaults.Add(serviceDefault)
		for _, split := range splits {
			splitters.Add(split)
			if split.Name != serviceDefault.Name {
				defaults.Add(httpServiceDefault(split, meta))
			}
		}

		return &api.IngressService{
			Name:      router.Name,
			Hosts:     utils.HostnamesForHTTPRoute(hostname, route),
			Namespace: "", // TODO
		}, router, splitters, defaults
	}
	return nil, nil, nil, nil
}

func (r *K8sRoute) Equals(other *K8sRoute) bool {
	if r == nil || other == nil {
		return false
	}

	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		if otherRoute, ok := other.Route.(*gw.HTTPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.TCPRoute:
		if otherRoute, ok := other.Route.(*gw.TCPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.UDPRoute:
		if otherRoute, ok := other.Route.(*gw.UDPRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	case *gw.TLSRoute:
		if otherRoute, ok := other.Route.(*gw.TLSRoute); ok {
			return reflect.DeepEqual(route.Spec, otherRoute.Spec)
		}
		return false
	}
	return false
}

func compareJSON(item interface{}) string {
	data, _ := json.Marshal(item)
	return string(data)
}
