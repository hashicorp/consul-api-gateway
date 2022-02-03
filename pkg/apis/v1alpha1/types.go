package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
)

const (
	GatewayClassConfigKind = "GatewayClassConfig"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// GatewayClassConfig describes the configuration of a consul-api-gateway GatewayClass.
type GatewayClassConfig struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of GatewayClassConfig.
	Spec GatewayClassConfigSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen=true

// GatewayClassConfigSpec specifies the 'spec' of the Config CRD.
type GatewayClassConfigSpec struct {
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	ServiceType *corev1.ServiceType `json:"serviceType,omitempty"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// If this is set, then the Envoy container ports are mapped
	// to host ports.
	UseHostPorts bool `json:"useHostPorts,omitempty"`
	// Configuration information about connecting to Consul.
	ConsulSpec ConsulSpec `json:"consul,omitempty"`
	// Configuration information about the images to use
	ImageSpec ImageSpec `json:"image,omitempty"`
	// Annotation Information to copy to services or deployments
	CopyAnnotations CopyAnnotationsSpec `json:"copyAnnotations,omitempty"`
	// +kubebuilder:validation:Enum=trace;debug;info;warning;error
	// Logging levels
	LogLevel string `json:"logLevel,omitempty"`
}

type ConsulSpec struct {
	// Consul authentication information
	AuthSpec AuthSpec `json:"authentication,omitempty"`
	// The scheme to use for connecting to Consul.
	// +kubebuilder:validation:Enum=http;https
	Scheme string `json:"scheme,omitempty"`
	// The address of the consul server to communicate with in the gateway
	// pod. If not specified, the pod will attempt to use a local agent on
	// the host on which it is running.
	Address string `json:"address,omitempty"`
	// The location of a secret to mount with the Consul root CA.
	CertificateAuthoritySecret string `json:"caSecret,omitempty"`
	// The information about Consul's ports
	PortSpec PortSpec `json:"ports,omitempty"`
}

type PortSpec struct {
	// The port for Consul's HTTP server.
	HTTP int `json:"http,omitempty"`
	// The grpc port for Consul's xDS server.
	GRPC int `json:"grpc,omitempty"`
}

type ImageSpec struct {
	// The image to use for consul-api-gateway.
	ConsulAPIGateway string `json:"consulAPIGateway,omitempty"`
	// The image to use for Envoy.
	Envoy string `json:"envoy,omitempty"`
}

//+kubebuilder:object:generate=true

type CopyAnnotationsSpec struct {
	// List of annotations to copy to the gateway service.
	Service []string `json:"service,omitempty"`
}

type AuthSpec struct {
	// Whether deployments should be run with "managed" service accounts created by the gateway controller.
	Managed bool `json:"managed,omitempty"`
	// The Consul auth method used for initial authentication by consul-api-gateway.
	Method string `json:"method,omitempty"`
	// The Kubernetes service account to authenticate as.
	Account string `json:"account,omitempty"`
	// The Consul namespace to use for authentication.
	Namespace string `json:"namespace,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayClassConfigList is a list of Config resources.
type GatewayClassConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []GatewayClassConfig `json:"items"`
}

// ServicesAccountFor returns the service account to be created for the given gateway.
func (c *GatewayClassConfig) ServiceAccountFor(gw *gateway.Gateway) *corev1.ServiceAccount {
	if !c.Spec.ConsulSpec.AuthSpec.Managed {
		return nil
	}
	labels := utils.LabelsForGateway(gw)
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gw.Name,
			Namespace: gw.Namespace,
			Labels:    labels,
		},
	}
}

// MergeService merges a gateway service a onto b and returns b, overriding all of
// the fields that we'd normally set for a service deployment. It does not attempt
// to change the service type
func MergeService(a, b *corev1.Service) *corev1.Service {
	if !compareServices(a, b) {
		a.Annotations = b.Annotations
		b.Spec.Ports = a.Spec.Ports
	}

	return b
}

func compareServices(a, b *corev1.Service) bool {
	// since K8s adds a bunch of defaults when we create a service, check that
	// they don't differ by the things that we may actually change, namely container
	// ports and propagated annotations
	if !equality.Semantic.DeepEqual(a.Annotations, b.Annotations) {
		return false
	}
	if len(b.Spec.Ports) != len(a.Spec.Ports) {
		return false
	}

	for i, port := range a.Spec.Ports {
		otherPort := b.Spec.Ports[i]
		if port.Port != otherPort.Port {
			return false
		}
		if port.Protocol != otherPort.Protocol {
			return false
		}
	}
	return true
}

// MergeDeploymentmerges a gateway deployment a onto b and returns b, overriding all of
// the fields that we'd normally set for a service deployment. It does not attempt
// to change the service type
func MergeDeployment(a, b *appsv1.Deployment) *appsv1.Deployment {
	if !compareDeployments(a, b) {
		b.Spec.Template = a.Spec.Template
	}

	return b
}

func compareDeployments(a, b *appsv1.Deployment) bool {
	// since K8s adds a bunch of defaults when we create a deployment, check that
	// they don't differ by the things that we may actually change, namely container
	// ports
	if len(b.Spec.Template.Spec.Containers) != len(a.Spec.Template.Spec.Containers) {
		return false
	}
	for i, container := range a.Spec.Template.Spec.Containers {
		otherPorts := b.Spec.Template.Spec.Containers[i].Ports
		if len(container.Ports) != len(otherPorts) {
			return false
		}
		for j, port := range container.Ports {
			otherPort := otherPorts[j]
			if port.ContainerPort != otherPort.ContainerPort {
				return false
			}
			if port.Protocol != otherPort.Protocol {
				return false
			}
		}
	}
	return true
}
