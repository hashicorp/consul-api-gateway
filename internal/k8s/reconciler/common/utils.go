package common

import (
	"encoding/json"

	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	supportedKindsForProtocol = map[gwv1beta1.ProtocolType][]gwv1beta1.RouteGroupKind{
		gwv1beta1.HTTPProtocolType: {{
			Group: (*gwv1beta1.Group)(&gwv1beta1.GroupVersion.Group),
			Kind:  "HTTPRoute",
		}},
		gwv1beta1.HTTPSProtocolType: {{
			Group: (*gwv1beta1.Group)(&gwv1beta1.GroupVersion.Group),
			Kind:  "HTTPRoute",
		}},
		gwv1beta1.TCPProtocolType: {{
			Group: (*gwv1beta1.Group)(&gwv1beta1.GroupVersion.Group),
			Kind:  "TCPRoute",
		}},
	}
)

// SupportedKindsFor --
func SupportedKindsFor(protocol gwv1beta1.ProtocolType) []gwv1beta1.RouteGroupKind {
	return supportedKindsForProtocol[protocol]
}

// AsJSON --
func AsJSON(item interface{}) string {
	data, err := json.Marshal(item)
	if err != nil {
		// everything passed to this internally should be
		// serializable, if something is passed to it that
		// isn't, just panic since it's a usage error at
		// that point
		panic(err)
	}
	return string(data)
}

// ParseParent --
func ParseParent(stringified string) gwv1alpha2.ParentReference {
	var ref gwv1alpha2.ParentReference
	if err := json.Unmarshal([]byte(stringified), &ref); err != nil {
		// everything passed to this internally should be
		// deserializable, if something is passed to it that
		// isn't, just panic since it's a usage error at
		// that point
		panic(err)
	}
	return ref
}
