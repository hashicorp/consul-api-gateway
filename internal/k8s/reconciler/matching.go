package reconciler

import (
	"errors"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var ErrInvalidNamespaceSelector = errors.New("invalid namespace selector")

func referencesGateway(namespace string, ref gw.ParentRef) (types.NamespacedName, bool) {
	gatewayGroup := gw.Group(gw.GroupName)
	gatewayKind := gw.Kind("Gateway")
	refGroup := gatewayGroup
	if ref.Group != nil {
		refGroup = *ref.Group
	}

	refKind := gatewayKind
	if ref.Kind != nil {
		refKind = *ref.Kind
	}

	if refGroup == gatewayGroup && refKind == gatewayKind {
		if ref.Namespace != nil {
			namespace = string(*ref.Namespace)
		}
		return types.NamespacedName{Name: ref.Name, Namespace: namespace}, true
	}
	return types.NamespacedName{}, false
}

func routeMatchesListener(listenerName gw.SectionName, sectionName *gw.SectionName) (bool, bool) {
	if sectionName == nil {
		return true, false
	}
	return listenerName == *sectionName, true
}

func routeMatchesListenerHostname(listenerHostname *gw.Hostname, hostnames []gw.Hostname) bool {
	if listenerHostname == nil || len(hostnames) == 0 {
		return true
	}

	for _, name := range hostnames {
		if utils.HostnamesMatch(name, *listenerHostname) {
			return true
		}
	}
	return false
}

func routeKindIsAllowedForListener(allowedRoutes *gw.AllowedRoutes, route *K8sRoute) bool {
	if allowedRoutes == nil {
		return true
	}

	gvk := route.GroupVersionKind()
	for _, kind := range allowedRoutes.Kinds {
		group := gw.GroupName
		if kind.Group != nil && *kind.Group != "" {
			group = string(*kind.Group)
		}
		if string(kind.Kind) == gvk.Kind && group == gvk.Group {
			return true
		}
	}

	return false
}

func routeAllowedForListenerNamespaces(gatewayNS string, allowedRoutes *gw.AllowedRoutes, route *K8sRoute) (bool, error) {
	// check gateway namespace
	namespaceSelector := allowedRoutes.Namespaces
	// set default is namespace selector is nil
	from := gw.NamespacesFromSame
	if namespaceSelector != nil && namespaceSelector.From != nil && *namespaceSelector.From != "" {
		from = *namespaceSelector.From
	}
	switch from {
	case gw.NamespacesFromAll:
		return true, nil
	case gw.NamespacesFromSame:
		return gatewayNS != route.GetNamespace(), nil
	case gw.NamespacesFromSelector:
		ns, err := metav1.LabelSelectorAsSelector(namespaceSelector.Selector)
		if err != nil {
			return false, ErrInvalidNamespaceSelector
		}

		return ns.Matches(toNamespaceSet(route.GetNamespace(), route.GetLabels())), nil
	}
	return false, nil
}

func toNamespaceSet(name string, labels map[string]string) klabels.Labels {
	// If namespace label is not set, implicitly insert it to support older Kubernetes versions
	if labels[NamespaceNameLabel] == name {
		// Already set, avoid copies
		return klabels.Set(labels)
	}
	// First we need a copy to not modify the underlying object
	ret := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		ret[k] = v
	}
	ret[NamespaceNameLabel] = name
	return klabels.Set(ret)
}
