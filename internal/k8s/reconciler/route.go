// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reconciler

import (
	"bytes"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/converter"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/store"
)

// Route represents any Kubernetes route type - currently v1alpha2.HTTPRoute
// and v1alpha2.TCPRoute - as both implement client.Object and schema.ObjectKind
type Route interface {
	client.Object
	schema.ObjectKind
}

type K8sRoute struct {
	Route
	RouteState *state.RouteState
}

type serializedRoute struct {
	GVK        schema.GroupVersionKind
	Route      []byte
	RouteState *state.RouteState
}

var _ store.Route = &K8sRoute{}

type K8sRouteConfig struct {
	ControllerName string
	Client         gatewayclient.Client
	State          *state.RouteState
}

func newK8sRoute(route Route, routeState *state.RouteState) *K8sRoute {
	return &K8sRoute{
		Route:      route,
		RouteState: routeState,
	}
}

func (r *K8sRoute) ID() string {
	switch r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return HTTPRouteID(utils.NamespacedName(r.Route))
	case *gwv1alpha2.TCPRoute:
		return TCPRouteID(utils.NamespacedName(r.Route))
	}
	return ""
}

func (r *K8sRoute) matchesHostname(hostname *gwv1beta1.Hostname) bool {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return routeMatchesListenerHostname(hostname, route.Spec.Hostnames)
	default:
		return true
	}
}

func (r *K8sRoute) commonRouteSpec() gwv1alpha2.CommonRouteSpec {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return route.Spec.CommonRouteSpec
	case *gwv1alpha2.TCPRoute:
		return route.Spec.CommonRouteSpec
	}
	return gwv1alpha2.CommonRouteSpec{}
}

func (r *K8sRoute) routeStatus() gwv1alpha2.RouteStatus {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return route.Status.RouteStatus
	case *gwv1alpha2.TCPRoute:
		return route.Status.RouteStatus
	}
	return gwv1alpha2.RouteStatus{}
}

func (r *K8sRoute) setStatus(updated gwv1alpha2.RouteStatus) {
	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		route.Status.RouteStatus = updated
	case *gwv1alpha2.TCPRoute:
		route.Status.RouteStatus = updated
	}
}

func (r *K8sRoute) resolve(namespace string, gateway *gwv1beta1.Gateway, listener gwv1beta1.Listener) core.ResolvedRoute {
	hostname := listenerHostname(listener)

	switch route := r.Route.(type) {
	case *gwv1alpha2.HTTPRoute:
		return converter.NewHTTPRouteConverter(converter.HTTPRouteConverterConfig{
			Namespace: namespace,
			Hostname:  hostname,
			Prefix:    fmt.Sprintf("consul-api-gateway_%s_", gateway.Name),
			Meta: map[string]string{
				"external-source":                            "consul-api-gateway",
				"consul-api-gateway/k8s/Gateway.Name":        gateway.Name,
				"consul-api-gateway/k8s/Gateway.Namespace":   gateway.Namespace,
				"consul-api-gateway/k8s/HTTPRoute.Name":      r.GetName(),
				"consul-api-gateway/k8s/HTTPRoute.Namespace": r.GetNamespace(),
			},
			Route: route,
			State: r.RouteState,
		}).Convert()
	case *gwv1alpha2.TCPRoute:
		return converter.NewTCPRouteConverter(converter.TCPRouteConverterConfig{
			Namespace: namespace,
			Hostname:  hostname,
			Prefix:    fmt.Sprintf("consul-api-gateway_%s_", gateway.Name),
			Meta: map[string]string{
				"external-source":                           "consul-api-gateway",
				"consul-api-gateway/k8s/Gateway.Name":       gateway.Name,
				"consul-api-gateway/k8s/Gateway.Namespace":  gateway.Namespace,
				"consul-api-gateway/k8s/TCPRoute.Name":      r.GetName(),
				"consul-api-gateway/k8s/TCPRoute.Namespace": r.GetNamespace(),
			},
			Route: route,
			State: r.RouteState,
		}).Convert()
	default:
		return nil
	}
}

func (r *K8sRoute) UnmarshalJSON(b []byte) error {
	stored := &serializedRoute{}
	if err := json.Unmarshal(b, stored); err != nil {
		return err
	}

	var into Route
	switch stored.GVK.Kind {
	case "HTTPRoute":
		into = &gwv1alpha2.HTTPRoute{}
	case "TCPRoute":
		into = &gwv1alpha2.TCPRoute{}
	}

	serializer := k8sjson.NewSerializer(k8sjson.DefaultMetaFactory, scheme, scheme, false)
	if _, _, err := serializer.Decode(stored.Route, &stored.GVK, into); err != nil {
		return err
	}

	r.Route = into
	r.RouteState = stored.RouteState

	return nil
}

func (r K8sRoute) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	serializer := k8sjson.NewSerializer(k8sjson.DefaultMetaFactory, scheme, scheme, false)
	if err := serializer.Encode(r.Route, &buffer); err != nil {
		return nil, err
	}

	return json.Marshal(&serializedRoute{
		GVK:        r.Route.GetObjectKind().GroupVersionKind(),
		Route:      buffer.Bytes(),
		RouteState: r.RouteState,
	})
}

func HTTPRouteID(namespacedName types.NamespacedName) string {
	return "http-" + namespacedName.String()
}

func TCPRouteID(namespacedName types.NamespacedName) string {
	return "tcp-" + namespacedName.String()
}
