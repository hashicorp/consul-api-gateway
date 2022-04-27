package reconciler

import (
	"bytes"
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"
	jsonruntime "k8s.io/apimachinery/pkg/runtime/serializer/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/store"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gw.AddToScheme(scheme))
}

func (r *K8sRoute) UnmarshalJSON(b []byte) error {
	stored := &storedRoute{}
	if err := json.Unmarshal(b, stored); err != nil {
		return err
	}
	var into Route
	switch stored.GVK.Kind {
	case "HTTPRoute":
		into = &gw.HTTPRoute{}
	case "TCPRoute":
		into = &gw.TCPRoute{}
	}
	serializer := jsonruntime.NewSerializer(jsonruntime.DefaultMetaFactory, scheme, scheme, false)
	if _, _, err := serializer.Decode(stored.Route, &stored.GVK, into); err != nil {
		return err
	}

	r.Route = into
	r.RouteState = stored.RouteState

	return nil
}

func (r K8sRoute) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	serializer := jsonruntime.NewSerializer(jsonruntime.DefaultMetaFactory, scheme, scheme, false)
	if err := serializer.Encode(r.Route, &buffer); err != nil {
		return nil, err
	}
	return json.Marshal(&storedRoute{
		GVK:        r.Route.GetObjectKind().GroupVersionKind(),
		Route:      buffer.Bytes(),
		RouteState: r.RouteState,
	})
}

type Marshaler struct{}

var _ store.Marshaler = &Marshaler{}

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
