package common

import (
	"encoding/json"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	supportedProtocols = map[gw.ProtocolType][]gw.RouteGroupKind{
		gw.HTTPProtocolType: {{
			Group: (*gw.Group)(&gw.GroupVersion.Group),
			Kind:  "HTTPRoute",
		}},
		gw.HTTPSProtocolType: {{
			Group: (*gw.Group)(&gw.GroupVersion.Group),
			Kind:  "HTTPRoute",
		}},
		gw.TCPProtocolType: {{
			Group: (*gw.Group)(&gw.GroupVersion.Group),
			Kind:  "TCPRoute",
		}},
	}
)

// SupportedKindsFor --
func SupportedKindsFor(protocol gw.ProtocolType) []gw.RouteGroupKind {
	return supportedProtocols[protocol]
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
func ParseParent(stringified string) gw.ParentRef {
	var ref gw.ParentRef
	if err := json.Unmarshal([]byte(stringified), &ref); err != nil {
		// everything passed to this internally should be
		// deserializable, if something is passed to it that
		// isn't, just panic since it's a usage error at
		// that point
		panic(err)
	}
	return ref
}
