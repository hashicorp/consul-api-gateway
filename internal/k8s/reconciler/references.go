package reconciler

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	routeReference resolvedReferenceType = iota
	consulServiceReference
)

type consulService struct {
	namespace string
	name      string
}

type resolvedReference struct {
	referenceType resolvedReferenceType
	ref           *backendRef
	object        client.Object
	consulService *consulService
}

type routeRuleReferenceMap map[RouteRule][]resolvedReference

func newConsulServiceReference(object client.Object) *resolvedReference {
	return &resolvedReference{
		referenceType: consulServiceReference,
		object:        object,
		ref:           &backendRef{},
	}
}

func (r *resolvedReference) SetConsul(service *consulService) *resolvedReference {
	r.consulService = service
	return r
}

func (r *resolvedReference) Item() client.Object {
	return r.object
}

type backendRef struct {
	httpRef *gw.HTTPBackendRef
}

func (b *backendRef) Set(ref interface{}) {
	switch backendRef := ref.(type) {
	case *gw.HTTPBackendRef:
		b.httpRef = backendRef
	}
}
