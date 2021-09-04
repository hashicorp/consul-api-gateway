package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GatewayClassConfig describes the configuration of a Polar GatewayClass.
// +k8s:openapi-gen=true
// +resource:path=resources
type GatewayClassConfig struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GatewayClassConfigSpec `json:"spec,omitempty"`
}

// GatewayClassConfigSpec specifies the 'spec' of the Config CRD.
type GatewayClassConfigSpec struct {
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	ServiceType  corev1.ServiceType  `json:"serviceType,omitempty"`
	NodeSelector corev1.NodeSelector `json:"nodeSelector,omitempty"`
	// If this is set, then the Envoy container ports are mapped
	// to host ports.
	UseHostPorts bool `json:"useHostPorts,omitempty"`
	// Configuration information about connecting to Consul.
	ConsulSpec ConsulSpec `json:"consul,omitempty"`
	// SDS server specific configuration
	SDSServerSpec SDSServerSpec `json:"sds,omitempty"`
	// Configuration information about the images to use
	ImageSpec ImageSpec `json:"image,omitempty"`
	// +kubebuilder:validation:Enum=trace;debug;info;warning;error
	// Logging levels
	LogLevel string `json:"logLevel,omitempty"`
}

type SDSServerSpec struct {
	// The SDS server address for Envoy secrets
	Address string `json:"address,omitempty"`
	// The SDS server port for Envoy secrets
	Port int `json:"port,omitempty"`
}

type ConsulSpec struct {
	// Consul authentication information
	AuthSpec AuthSpec `json:"authentication,omitempty"`
	// The scheme to use for connecting to Consul.
	Scheme string `json:"scheme,omitempty"`
	// The address of the consul server to communicate with in the gateway
	// pod. If not specified, the pod will attempt to use a local agent on
	// the host on which it is running.
	Address string `json:"address,omitempty"`
	// The information about Consul's ports
	PortSpec PortSpec `json:"ports,omitempty"`
	// The location of a secret to mount with the Consul root CA.
	CASecret string `json:"caSecret,omitempty"`
}

type PortSpec struct {
	// The port for Consul's HTTP server.
	HTTP int `json:"http,omitempty"`
	// The grpc port for Consul's xDS server.
	GRPC int `json:"grpc,omitempty"`
}

type ImageSpec struct {
	// The image to use for Polar.
	Polar string `json:"polar,omitempty"`
	// The image to use for Envoy.
	Envoy string `json:"envoy,omitempty"`
}

type AuthSpec struct {
	// The Consul auth method used for initial authentication by Polar.
	Method string `json:"method,omitempty"`
	// The Kubernetes service account to authenticate as.
	Account string `json:"account,omitempty"`
	// The Consul namespace to use for authentication.
	Namespace string `json:"namespace,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayClassConfigList is a list of Config resources.
type GatewayClassConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []GatewayClassConfig `json:"items"`
}
