package consul

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (r *KubernetesRoutes) HTTPRoutes() []*gw.HTTPRoute {
	r.lock.Lock()
	defer r.lock.Unlock()
	var routes []*gw.HTTPRoute
	for _, v := range r.routes {
		route, ok := v.(*gw.HTTPRoute)
		if ok {
			routes = append(routes, route)
		}
	}
	return routes
}
