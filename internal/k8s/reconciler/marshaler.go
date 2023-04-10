// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reconciler

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/store"
)

var (
	_ store.Marshaler = (*Marshaler)(nil)

	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gwv1alpha2.AddToScheme(scheme))
}

type Marshaler struct{}

func NewMarshaler() *Marshaler {
	return &Marshaler{}
}

func (m *Marshaler) UnmarshalRoute(data []byte) (store.Route, error) {
	route := &K8sRoute{}
	if err := json.Unmarshal(data, route); err != nil {
		return nil, err
	}
	return route, nil
}

func (m *Marshaler) MarshalRoute(route store.Route) ([]byte, error) {
	return json.Marshal(route)
}

func (m *Marshaler) UnmarshalGateway(data []byte) (store.Gateway, error) {
	gateway := &K8sGateway{}
	if err := json.Unmarshal(data, gateway); err != nil {
		return nil, err
	}
	return gateway, nil
}

func (m *Marshaler) MarshalGateway(gateway store.Gateway) ([]byte, error) {
	return json.Marshal(gateway)
}
