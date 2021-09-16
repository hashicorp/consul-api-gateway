package v1alpha1

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/polar/version"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	defaultImage string
)

func init() {
	defaultImage = fmt.Sprintf("hashicorp/polar:%s", version.Version)
}

const (
	GatewayClassConfigKind = "GatewayClassConfig"

	defaultEnvoyImage     = "envoyproxy/envoy:v1.19-latest"
	defaultLogLevel       = "info"
	defaultCASecret       = "consul-ca-cert"
	defaultConsulAddress  = "$(HOST_IP)"
	defaultConsulHTTPPort = "8500"
	defaultConsulXDSPort  = "8502"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// GatewayClassConfig describes the configuration of a Polar GatewayClass.
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

// +kubebuilder:object:root=true

// GatewayClassConfigList is a list of Config resources.
type GatewayClassConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []GatewayClassConfig `json:"items"`
}

type SDSConfig struct {
	Host string
	Port int
}

// ServicesFor returns the service configuration for the given gateway.
// The gateway should be marked with the polar.hashicorp.com/service-type
// annotation and marked with 'ClusterIP', `NodePort` or `LoadBalancer` to
// expose the gateway listeners. Any other value does not expose the gateway.
func (c *GatewayClassConfig) ServiceFor(gw *gateway.Gateway) *corev1.Service {
	if c.Spec.ServiceType == nil {
		return nil
	}
	ports := []corev1.ServicePort{}
	for _, listener := range gw.Spec.Listeners {
		ports = append(ports, corev1.ServicePort{
			Name:     listener.Name,
			Protocol: "TCP",
			Port:     int32(listener.Port),
		})
	}
	labels := labelsFor(gw)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gw.Name,
			Namespace: gw.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     c.serviceType(),
			Ports:    ports,
		},
	}
}

func (c *GatewayClassConfig) serviceType() corev1.ServiceType {
	if c.Spec.ServiceType == nil {
		return ""
	}
	return *c.Spec.ServiceType
}

// DeploymentsFor returns the deployment configuration for the given gateway.
func (c *GatewayClassConfig) DeploymentFor(gw *gateway.Gateway, sds SDSConfig) *appsv1.Deployment {
	labels := labelsFor(gw)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gw.Name,
			Namespace: gw.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: c.podSpecFor(gw, sds),
			},
		},
	}
}

func (c *GatewayClassConfig) podSpecFor(gw *gateway.Gateway, sds SDSConfig) corev1.PodSpec {
	volumes, mounts := c.volumesFor(gw)
	return corev1.PodSpec{
		NodeSelector:       c.Spec.NodeSelector,
		ServiceAccountName: orDefault(c.Spec.ConsulSpec.AuthSpec.Account, ""),
		// the init container copies the binary into the
		// next envoy container so we can decouple the envoy
		// versions from our version of polar.
		InitContainers: []corev1.Container{{
			Image:        orDefault(c.Spec.ImageSpec.Polar, defaultImage),
			Name:         "polar-init",
			VolumeMounts: mounts,
			Command: []string{
				"cp", "/bin/polar", "/bootstrap/polar",
			},
		}},
		Containers: []corev1.Container{{
			Image:        orDefault(c.Spec.ImageSpec.Envoy, defaultEnvoyImage),
			Name:         "polar",
			VolumeMounts: mounts,
			Ports:        c.containerPortsFor(gw),
			Env: []corev1.EnvVar{
				{
					Name: "IP",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "status.podIP",
						},
					},
				},
				{
					Name: "HOST_IP",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "status.hostIP",
						},
					},
				},
			},
			Command: c.execCommandFor(gw, sds),
		}},
		Volumes: volumes,
	}
}

func (c *GatewayClassConfig) execCommandFor(gw *gateway.Gateway, sds SDSConfig) []string {
	initCommand := []string{
		"/bootstrap/polar", "exec",
		"-log-json",
		"-log-level", orDefault(c.Spec.LogLevel, defaultLogLevel),
		"-gateway-host", "$(IP)",
		"-gateway-name", gw.Name,
		"-consul-http-address", orDefault(c.Spec.ConsulSpec.Address, defaultConsulAddress),
		"-consul-http-port", orDefaultIntString(c.Spec.ConsulSpec.PortSpec.HTTP, defaultConsulHTTPPort),
		"-consul-xds-port", orDefaultIntString(c.Spec.ConsulSpec.PortSpec.GRPC, defaultConsulXDSPort),
		"-envoy-bootstrap-path", "/bootstrap/envoy.json",
		"-envoy-sds-address", sds.Host,
		"-envoy-sds-port", strconv.Itoa(sds.Port),
	}

	if method := c.Spec.ConsulSpec.AuthSpec.Method; method != "" {
		initCommand = append(initCommand, "-acl-auth-method", method)
	}

	if c.requiresCA(gw) {
		initCommand = append(initCommand, "-consul-ca-cert-file", "/ca/tls.crt")
	}
	return initCommand
}

func (c *GatewayClassConfig) volumesFor(gw *gateway.Gateway) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{{
		Name: "bootstrap",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}, {
		Name: "certs",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}}
	mounts := []corev1.VolumeMount{{
		Name:      "bootstrap",
		MountPath: "/bootstrap",
	}, {
		Name:      "certs",
		MountPath: "/certs",
	}}
	if c.requiresCA(gw) {
		caCertSecret := orDefault(c.Spec.ConsulSpec.CASecret, defaultCASecret)
		volumes = append(volumes, corev1.Volume{
			Name: "ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: caCertSecret,
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "ca",
			MountPath: "/ca",
			ReadOnly:  true,
		})
	}
	return volumes, mounts
}

func orDefault(value, defaultValue string) string {
	if value != "" {
		return value
	}
	return defaultValue
}

func orDefaultIntString(value int, defaultValue string) string {
	if value != 0 {
		return strconv.Itoa(value)
	}
	return defaultValue
}

func (c *GatewayClassConfig) containerPortsFor(gw *gateway.Gateway) []corev1.ContainerPort {
	ports := []corev1.ContainerPort{{
		Name:          "ready",
		Protocol:      "TCP",
		ContainerPort: 20000,
	}}
	for _, listener := range gw.Spec.Listeners {
		port := corev1.ContainerPort{
			Name:          listener.Name,
			Protocol:      "TCP",
			ContainerPort: int32(listener.Port),
		}
		if c.Spec.UseHostPorts {
			port.HostPort = int32(listener.Port)
		}
		ports = append(ports, port)
	}
	return ports
}

func (c *GatewayClassConfig) requiresCA(gw *gateway.Gateway) bool {
	return c.Spec.ConsulSpec.Scheme == "https"
}

func labelsFor(gw *gateway.Gateway) map[string]string {
	return map[string]string{
		"name":      "polar-" + gw.Name,
		"managedBy": "polar",
	}
}
