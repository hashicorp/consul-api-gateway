package builder

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

type GatewayServiceBuilder struct {
	gateway  *gw.Gateway
	gwConfig *v1alpha1.GatewayClassConfig
}

func NewGatewayService(gw *gw.Gateway) *GatewayServiceBuilder {
	return &GatewayServiceBuilder{gateway: gw}
}

func (b *GatewayServiceBuilder) WithClassConfig(cfg v1alpha1.GatewayClassConfig) {
	b.gwConfig = &cfg
}

func (b *GatewayServiceBuilder) Validate() error {
	if b.gwConfig == nil {
		return fmt.Errorf("GatewayClassConfig must be set")
	}

	return nil
}
func (b *GatewayServiceBuilder) Build() *corev1.Service {
	if b.gwConfig.Spec.ServiceType == nil {
		return nil
	}
	ports := []corev1.ServicePort{}
	for _, listener := range b.gateway.Spec.Listeners {
		ports = append(ports, corev1.ServicePort{
			Name:     string(listener.Name),
			Protocol: "TCP",
			Port:     int32(listener.Port),
		})
	}
	labels := utils.LabelsForGateway(b.gateway)
	allowedAnnotations := b.gwConfig.Spec.CopyAnnotations.Service
	if allowedAnnotations == nil {
		allowedAnnotations = defaultServiceAnnotations
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.gateway.Name,
			Namespace:   b.gateway.Namespace,
			Labels:      labels,
			Annotations: filterAnnotations(b.gateway.Annotations, allowedAnnotations),
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     *b.gwConfig.Spec.ServiceType,
			Ports:    ports,
		},
	}
}

func filterAnnotations(annotations map[string]string, allowed []string) map[string]string {
	filtered := make(map[string]string)
	for _, annotation := range allowed {
		if value, found := annotations[annotation]; found {
			filtered[annotation] = value
		}
	}
	return filtered
}

type GatewayDeploymentBuilder struct {
	gateway      *gw.Gateway
	gwConfig     *v1alpha1.GatewayClassConfig
	sdsHost      string
	sdsPort      int
	consulCAData string
}

func NewGatewayDeployment(gw *gw.Gateway) *GatewayDeploymentBuilder {
	return &GatewayDeploymentBuilder{gateway: gw}
}

func (b *GatewayDeploymentBuilder) WithClassConfig(cfg v1alpha1.GatewayClassConfig) {
	b.gwConfig = &cfg
}

func (b *GatewayDeploymentBuilder) WithSDS(host string, port int) {
	b.sdsHost = host
	b.sdsPort = port
}

func (b *GatewayDeploymentBuilder) WithConsulCA(caData string) {
	b.consulCAData = caData
}

func (b *GatewayDeploymentBuilder) Validate() error {
	if b.gwConfig == nil {
		return fmt.Errorf("GatewayClassConfig must be set")
	}

	if b.sdsHost == "" || b.sdsPort == 0 {
		return fmt.Errorf("SDS must be set")
	}

	if b.requiresCA() && b.consulCAData == "" {
		return fmt.Errorf("ConsulCA must be set")
	}
	return nil
}

func (b *GatewayDeploymentBuilder) Build() *v1.Deployment {
	labels := utils.LabelsForGateway(b.gateway)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.gateway.Name,
			Namespace: b.gateway.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"consul.hashicorp.com/connect-inject": "false",
					},
				},
				Spec: b.podSpec(),
			},
		},
	}
}

func (b *GatewayDeploymentBuilder) podSpec() corev1.PodSpec {
	volumes, mounts := b.volumes()
	return corev1.PodSpec{
		NodeSelector:       b.gwConfig.Spec.NodeSelector,
		ServiceAccountName: orDefault(b.gwConfig.Spec.ConsulSpec.AuthSpec.Account, ""),
		// the init container copies the binary into the
		// next envoy container so we can decouple the envoy
		// versions from our version of consul-api-gateway.
		InitContainers: []corev1.Container{{
			Image:        orDefault(b.gwConfig.Spec.ImageSpec.ConsulAPIGateway, defaultImage),
			Name:         "consul-api-gateway-init",
			VolumeMounts: mounts,
			Command: []string{
				"cp", "/bin/consul-api-gateway", "/bootstrap/consul-api-gateway",
			},
		}},
		Containers: []corev1.Container{{
			Image:        orDefault(b.gwConfig.Spec.ImageSpec.Envoy, defaultEnvoyImage),
			Name:         "consul-api-gateway",
			VolumeMounts: mounts,
			Ports:        b.containerPorts(),
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
			Command: b.execCommand(),
			ReadinessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/ready",
						Port: intstr.FromInt(20000),
					},
				},
			},
		}},
		Volumes: volumes,
	}
}

func (b *GatewayDeploymentBuilder) execCommand() []string {
	// Render the command
	data := gwContainerCommandData{
		ConsulHTTPAddr: orDefault(b.gwConfig.Spec.ConsulSpec.Address, defaultConsulAddress),
		ConsulHTTPPort: orDefaultIntString(b.gwConfig.Spec.ConsulSpec.PortSpec.HTTP, defaultConsulHTTPPort),
		ConsulGRPCPort: orDefaultIntString(b.gwConfig.Spec.ConsulSpec.PortSpec.GRPC, defaultConsulXDSPort),
		LogLevel:       orDefault(b.gwConfig.Spec.LogLevel, defaultLogLevel),
		GatewayHost:    "$(IP)",
		GatewayName:    b.gateway.Name,
		SDSHost:        b.sdsHost,
		SDSPort:        b.sdsPort,
	}
	if b.requiresCA() {
		data.ConsulCAFile = consulCALocalFile
		data.ConsulCAData = b.consulCAData
	}
	if method := b.gwConfig.Spec.ConsulSpec.AuthSpec.Method; method != "" {
		data.ACLAuthMethod = method
	}
	var buf bytes.Buffer
	err := template.Must(template.New("root").Parse(strings.TrimSpace(
		gwContainerCommandTpl))).Execute(&buf, &data)
	if err != nil {
		return nil
	}

	return []string{"/bin/sh", "-ec", buf.String()}
}

func (b *GatewayDeploymentBuilder) volumes() ([]corev1.Volume, []corev1.VolumeMount) {
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
	if b.requiresCA() {
		volumes = append(volumes, corev1.Volume{
			Name: "ca",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "ca",
			MountPath: consulCALocalPath,
			ReadOnly:  true,
		})
	}
	return volumes, mounts
}

func (b *GatewayDeploymentBuilder) containerPorts() []corev1.ContainerPort {
	ports := []corev1.ContainerPort{{
		Name:          "ready",
		Protocol:      "TCP",
		ContainerPort: 20000,
	}}
	for _, listener := range b.gateway.Spec.Listeners {
		port := corev1.ContainerPort{
			Name:          string(listener.Name),
			Protocol:      "TCP",
			ContainerPort: int32(listener.Port),
		}
		if b.gwConfig.Spec.UseHostPorts {
			port.HostPort = int32(listener.Port)
		}
		ports = append(ports, port)
	}
	return ports
}

func (b *GatewayDeploymentBuilder) requiresCA() bool {
	return b.gwConfig.Spec.ConsulSpec.Scheme == "https"
}

type gwContainerCommandData struct {
	ConsulCAFile   string
	ConsulCAData   string
	ConsulHTTPAddr string
	ConsulHTTPPort string
	ConsulGRPCPort string
	ACLAuthMethod  string
	LogLevel       string
	GatewayHost    string
	GatewayName    string
	SDSHost        string
	SDSPort        int
}

// gwContainerCommandTpl is the template for the command executed by
// the exec container.
const gwContainerCommandTpl = `
{{- if .ConsulCAFile}}
export CONSUL_CACERT={{ .ConsulCAFile }}
cat <<EOF >{{ .ConsulCAFile }}
{{ .ConsulCAData }}
EOF
{{- end}}

/bootstrap/consul-api-gateway exec -log-json \
  -log-level {{ .LogLevel }} \
  -gateway-host "{{ .GatewayHost }}" \
  -gateway-name {{ .GatewayName }} \
  -consul-http-address {{ .ConsulHTTPAddr }} \
  -consul-http-port {{ .ConsulHTTPPort }} \
  -consul-xds-port  {{ .ConsulGRPCPort }} \
{{- if .ACLAuthMethod }}
  -acl-auth-method {{ .ACLAuthMethod }} \
{{- end }}
  -envoy-bootstrap-path /bootstrap/envoy.json \
  -envoy-sds-address {{ .SDSHost }} \
  -envoy-sds-port {{ .SDSPort }}
`
