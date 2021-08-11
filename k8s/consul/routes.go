package consul

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type KubernetesRoutes struct {
	routes map[types.NamespacedName]metav1.Object
	lock   sync.Mutex
}

func NewKubernetesRoutes() *KubernetesRoutes {
	return &KubernetesRoutes{routes: map[types.NamespacedName]metav1.Object{}}
}

func (r *KubernetesRoutes) Set(val metav1.Object) {
	r.lock.Lock()
	r.routes[kubeObjectNamespacedName(val)] = val
	r.lock.Unlock()
}

func (r *KubernetesRoutes) Delete(name types.NamespacedName) {
	r.lock.Lock()
	delete(r.routes, name)
	r.lock.Unlock()
}

// HTTPRoutes returns a slice of copies of routes which are of the HTTPRoute type
func (r *KubernetesRoutes) HTTPRoutes() []*gw.HTTPRoute {
	r.lock.Lock()
	defer r.lock.Unlock()
	var routes []*gw.HTTPRoute
	for _, v := range r.routes {
		route, ok := v.(*gw.HTTPRoute)
		if ok {
			routes = append(routes, route.DeepCopy())
		}
	}
	return routes
}

type kubeObj interface {
	schema.ObjectKind
	metav1.Object
}

// routeMatches implements the logic to determine if a route should be associated to the given gateway by checking it
// against a gateway listener's route binding selector.
func routeMatches(gateway *gw.Gateway, selector gw.RouteBindingSelector, route kubeObj) (bool, string) {
	gvk := route.GroupVersionKind()
	// check selector kind and group
	if selector.Kind != gvk.Kind {
		return false, "selector and route Kind do not match"
	}

	group := gw.GroupName // default
	if selector.Group != nil && *selector.Group != "" {
		group = *selector.Group
	}
	if group != gvk.Group {
		return false, "selector and route Group do not match"
	}

	// check gateway labels
	var labelSelector klabels.Selector
	var err error
	if selector.Selector == nil {
		labelSelector = klabels.Everything()
	} else {
		labelSelector, err = metav1.LabelSelectorAsSelector(selector.Selector)
		if err != nil {
			return false, "bad selector"
		}
	}

	if !labelSelector.Matches(klabels.Set(route.GetLabels())) {
		return false, "gateway labels selector does not match route"
	}

	// check gateway namespace
	namespaceSelector := selector.Namespaces
	// set default is namespace selector is nil
	from := gw.RouteSelectSame
	if namespaceSelector != nil && namespaceSelector.From != nil && *namespaceSelector.From != "" {
		from = *namespaceSelector.From
	}
	switch from {
	case gw.RouteSelectAll:
	// matches continue
	case gw.RouteSelectSame:
		if gateway.Namespace != route.GetNamespace() {
			return false, "gateway namespace does not match route"
		}
	case gw.RouteSelectSelector:
		ns, err := metav1.LabelSelectorAsSelector(namespaceSelector.Selector)
		if err != nil {
			return false, "bad selector"
		}

		if !ns.Matches(toNamespaceSet(route.GetNamespace(), route.GetLabels())) {
			return false, "gateway namespace does not match route namespace selector"
		}

	}

	// check gateway selector
	gatewaySelector := getRouteGatewaysFromRoute(route)
	allow := gw.GatewayAllowSameNamespace
	if gatewaySelector != nil && gatewaySelector.Allow != nil {
		allow = *gatewaySelector.Allow
	}

	switch allow {
	case gw.GatewayAllowAll:
	// matches
	case gw.GatewayAllowFromList:
		found := false
		if gatewaySelector == nil {
			return false, "route gateway selector is empty but gateway requires allow from list"
		}
		for _, gw := range gatewaySelector.GatewayRefs {
			if gw.Name == gateway.Name && gw.Namespace == gateway.Namespace {
				found = true
				break
			}
		}
		if !found {
			return false, "route gateway selector did not match"
		}
	case gw.GatewayAllowSameNamespace:
		if gateway.Namespace != route.GetNamespace() {
			return false, "gateway namespace does not match and is required by gateway selector"
		}
	}

	return true, ""
}

func getRouteGatewaysFromRoute(route interface{}) *gw.RouteGateways {
	switch r := route.(type) {
	case *gw.HTTPRoute:
		return r.Spec.Gateways
	default:
		return nil
	}
}
