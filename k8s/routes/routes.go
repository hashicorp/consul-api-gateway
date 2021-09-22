package routes

import (
	"fmt"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/k8s/utils"
)

// all kubernetes routes implement the following two interfaces
type Route interface {
	client.Object
	schema.ObjectKind
}

type KubernetesRoute struct {
	Route
}

func (r *KubernetesRoute) IsHTTPRoute() bool {
	_, ok := r.Route.(*gw.HTTPRoute)
	return ok
}

func (r *KubernetesRoute) AsHTTPRoute() (*gw.HTTPRoute, bool) {
	val, ok := r.Route.(*gw.HTTPRoute)
	if !ok {
		return nil, false
	}
	return val.DeepCopy(), true
}

func (r *KubernetesRoute) CommonRouteSpec() gw.CommonRouteSpec {
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

func (r *KubernetesRoute) RouteStatus() gw.RouteStatus {
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

func (r *KubernetesRoute) SetStatus(status gw.RouteStatus) {
	switch route := r.Route.(type) {
	case *gw.HTTPRoute:
		route.Status.RouteStatus = status
	case *gw.TCPRoute:
		route.Status.RouteStatus = status
	case *gw.UDPRoute:
		route.Status.RouteStatus = status
	case *gw.TLSRoute:
		route.Status.RouteStatus = status
	}
}

func (r *KubernetesRoute) IsAdmittedByGatewayListener(gatewayName types.NamespacedName, routes *gw.AllowedRoutes) (admitted bool, reason, message string) {
	gvk := r.GroupVersionKind()
	// check selector kind and group

	if len(routes.Kinds) > 0 {
		gkMatch := false
		for _, rgk := range routes.Kinds {
			group := gw.GroupName
			if rgk.Group != nil && *rgk.Group != "" {
				group = string(*rgk.Group)
			}
			if string(rgk.Kind) == gvk.Kind && group == gvk.Group {
				gkMatch = true
				break
			}
		}
		if !gkMatch {
			return false, "InvalidRoutesRef", "route does not match listener's allowed groups and kinds"
		}
	}

	// check gateway namespace
	namespaceSelector := routes.Namespaces
	// set default is namespace selector is nil
	from := gw.NamespacesFromSame
	if namespaceSelector != nil && namespaceSelector.From != nil && *namespaceSelector.From != "" {
		from = *namespaceSelector.From
	}
	switch from {
	case gw.NamespacesFromAll:
	// matches continue
	case gw.NamespacesFromSame:
		if gatewayName.Namespace != r.GetNamespace() {
			return false, "InvalidRoutesRef", "gateway namespace does not match route"
		}
	case gw.NamespacesFromSelector:
		ns, err := metav1.LabelSelectorAsSelector(namespaceSelector.Selector)
		if err != nil {
			return false, "InvalidRoutesRef", "namespace selector could not be parsed"
		}

		if !ns.Matches(toNamespaceSet(r.GetNamespace(), r.GetLabels())) {
			return false, "InvalidRoutesRef", "gateway namespace does not match route namespace selector"
		}

	}
	return true, "", ""
}

func (r *KubernetesRoute) ParentRefAllowed(ref gw.ParentRef, gatewayName types.NamespacedName, listener gw.Listener) error {
	// First check if any hostnames match the listener for HTTPRoutes. The spec states that even if a parent can be referenced, the route
	// cannot be admitted unless one of the host names match the listener's Hostname. If the route or listener Hostnames are not
	// set, '*' is assumed which allows all.
	if r.IsHTTPRoute() {
		route, _ := r.AsHTTPRoute()
		if len(route.Spec.Hostnames) > 0 && listener.Hostname != nil {
			var match bool
			for _, name := range route.Spec.Hostnames {
				if hostnamesMatch(name, *listener.Hostname) {
					match = true
					break
				}
			}

			if !match {
				return fmt.Errorf("no listeners had matching Hostnames")
			}
		}
	}

	return routeParentRefMatches(ref, gatewayName, string(listener.Name), r.GetNamespace())
}

type KubernetesRoutes struct {
	routes map[types.NamespacedName]*KubernetesRoute
	lock   sync.Mutex
}

func NewKubernetesRoutes() *KubernetesRoutes {
	return &KubernetesRoutes{
		routes: map[types.NamespacedName]*KubernetesRoute{},
	}
}

func (r *KubernetesRoutes) Set(route Route) bool {
	name := utils.KubeObjectNamespacedName(route)
	r.lock.Lock()
	defer r.lock.Unlock()
	cur, ok := r.routes[name]
	if ok && cur.Route.GetGeneration() == route.GetGeneration() {
		return false
	}
	r.routes[name] = &KubernetesRoute{route}
	return true
}

func (r *KubernetesRoutes) Delete(name types.NamespacedName) {
	r.lock.Lock()
	delete(r.routes, name)
	r.lock.Unlock()
}

// HTTPRoutes returns a slice of KubernetesRoute pointers which are of kind HTTPRoute
func (r *KubernetesRoutes) HTTPRoutes() []*KubernetesRoute {
	r.lock.Lock()
	defer r.lock.Unlock()
	var routes []*KubernetesRoute
	for _, v := range r.routes {
		if v.IsHTTPRoute() {
			routes = append(routes, v)
		}
	}
	return routes
}

func routeParentRefMatches(ref gw.ParentRef, gatewayName types.NamespacedName, listenerName, localNamespace string) error {
	// only match gateway.networking.k8s.io group for now
	if ref.Group != nil && *ref.Group != gw.GroupName {
		return fmt.Errorf("no matching parents with group: %s", *ref.Group)
	}

	// only match gateway references
	if ref.Kind != nil && *ref.Kind != "Gateway" {
		return fmt.Errorf("no matching parents with kind: %s", *ref.Kind)
	}

	// match gateway namesapce
	namespace := localNamespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		namespace = string(*ref.Namespace)
	}
	if gatewayName.Namespace != namespace {
		return fmt.Errorf("no matching parents with namespace: %s", namespace)
	}

	if ref.Name != gatewayName.Name {
		return fmt.Errorf("no matching parents with name: %s", ref.Name)
	}

	if listenerName != "" && ref.SectionName != nil && string(*ref.SectionName) != listenerName {
		return fmt.Errorf("no matching parent sections with name: %s", *ref.SectionName)
	}

	return nil
}

func hostnamesMatch(a, b gw.Hostname) bool {
	if a == "" || a == "*" || b == "" || b == "*" {
		// any wildcard always matches
		return true
	}

	if strings.HasPrefix(string(a), "*.") || strings.HasPrefix(string(b), "*.") {
		aLabels, bLabels := strings.Split(string(a), "."), strings.Split(string(b), ".")
		if len(aLabels) != len(bLabels) {
			return false
		}

		for i := 1; i < len(aLabels); i++ {
			if !strings.EqualFold(aLabels[i], bLabels[i]) {
				return false
			}
		}
	}

	return a == b

}
