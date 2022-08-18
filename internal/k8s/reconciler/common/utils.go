package common

import (
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
