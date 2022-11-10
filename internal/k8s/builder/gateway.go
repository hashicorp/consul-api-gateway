package builder

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/hashicorp/consul/api"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

type GatewayServiceBuilder struct {
	gateway  *gwv1beta1.Gateway
	gwConfig *v1alpha1.GatewayClassConfig
}

func NewGatewayService(gw *gwv1beta1.Gateway) *GatewayServiceBuilder {
	return &GatewayServiceBuilder{gateway: gw}
}

func (b *GatewayServiceBuilder) WithClassConfig(cfg v1alpha1.GatewayClassConfig) *GatewayServiceBuilder {
	b.gwConfig = &cfg
	return b
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
	gateway                 *gwv1beta1.Gateway
	gwConfig                *v1alpha1.GatewayClassConfig
	sdsHost                 string
	sdsPort                 int
	consulCAData            string
	consulGatewayNamespace  string
	consulPrimaryDatacenter string
}

func NewGatewayDeployment(gw *gwv1beta1.Gateway) *GatewayDeploymentBuilder {
	return &GatewayDeploymentBuilder{gateway: gw}
}

func (b *GatewayDeploymentBuilder) WithClassConfig(cfg v1alpha1.GatewayClassConfig) *GatewayDeploymentBuilder {
	b.gwConfig = &cfg
	return b
}

func (b *GatewayDeploymentBuilder) WithSDS(host string, port int) *GatewayDeploymentBuilder {
	b.sdsHost = host
	b.sdsPort = port
	return b
}

func (b *GatewayDeploymentBuilder) WithConsulCA(caData string) *GatewayDeploymentBuilder {
	b.consulCAData = caData
	return b
}

func (b *GatewayDeploymentBuilder) WithConsulGatewayNamespace(namespace string) *GatewayDeploymentBuilder {
	b.consulGatewayNamespace = namespace
	return b
}

func (b *GatewayDeploymentBuilder) WithPrimaryConsulDatacenter(datacenter string) *GatewayDeploymentBuilder {
	b.consulPrimaryDatacenter = datacenter
	return b
}

func (b *GatewayDeploymentBuilder) Build(currentReplicas *int32) *v1.Deployment {
	labels := utils.LabelsForGateway(b.gateway)

	return &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.gateway.Name,
			Namespace: b.gateway.Namespace,
			Labels:    labels,
		},
		Spec: v1.DeploymentSpec{
			Replicas: b.instances(currentReplicas),
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

func (b *GatewayDeploymentBuilder) instances(currentReplicas *int32) *int32 {

	instanceValue := defaultInstances

	//if currentReplicas is not nil use current value when building deployment
	if currentReplicas != nil {
		instanceValue = *currentReplicas
	} else if b.gwConfig.Spec.DeploymentSpec.DefaultInstances != nil {
		// otherwise use the default value on the GatewayClassConfig if set
		instanceValue = *b.gwConfig.Spec.DeploymentSpec.DefaultInstances
	}

	if b.gwConfig.Spec.DeploymentSpec.MaxInstances != nil {

		//check if over maximum and lower to maximum
		maxValue := *b.gwConfig.Spec.DeploymentSpec.MaxInstances
		if instanceValue > maxValue {
			instanceValue = maxValue
		}
	}

	if b.gwConfig.Spec.DeploymentSpec.MinInstances != nil {
		//check if less than minimum and raise to minimum
		minValue := *b.gwConfig.Spec.DeploymentSpec.MinInstances
		if instanceValue < minValue {
			instanceValue = minValue
		}

	}
	return &instanceValue
}

func (b *GatewayDeploymentBuilder) podSpec() corev1.PodSpec {
	volumes, mounts := b.volumes()
	defaultServiceAccount := ""
	if b.gwConfig.Spec.ConsulSpec.AuthSpec.Managed {
		defaultServiceAccount = b.gateway.Name
	}

	labels := utils.LabelsForGateway(b.gateway)

	return corev1.PodSpec{
		Affinity: &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					{
						Weight: 1,
						PodAffinityTerm: corev1.PodAffinityTerm{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: labels,
							},
							TopologyKey: k8sHostnameTopologyKey,
						},
					},
				},
			},
		},
		NodeSelector:       b.gwConfig.Spec.NodeSelector,
		Tolerations:        b.gwConfig.Spec.Tolerations,
		ServiceAccountName: orDefault(b.gwConfig.Spec.ConsulSpec.AuthSpec.Account, defaultServiceAccount),
		// the init container copies the binary into the
		// next envoy container so we can decouple the envoy
		// versions from our version of consul-api-gateway.

		InitContainers: []corev1.Container{{
			Image:        orDefault(b.gwConfig.Spec.ImageSpec.ConsulAPIGateway, defaultImage),
			Name:         "consul-api-gateway-init",
			VolumeMounts: mounts,
			Command: []string{
				"cp", "/bin/discover", "/bin/consul-api-gateway", "/bootstrap/",
			},
		}},
		Containers: []corev1.Container{{
			Image:        orDefault(b.gwConfig.Spec.ImageSpec.Envoy, defaultEnvoyImage),
			Name:         "consul-api-gateway",
			VolumeMounts: mounts,
			Ports:        b.containerPorts(),
			Env:          b.envVars(),
			Command:      []string{"/bootstrap/consul-api-gateway", "exec"},
			Args:         b.execArgs(),
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
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

// execArgs renders a template containing all necessary args for the container
// command. Due to the format expected by the K8s API/SDK, this template is then
// split into a []string where each line of the template is its own item.
func (b *GatewayDeploymentBuilder) execArgs() []string {
	data := gwContainerCommandData{
		ACLAuthMethod:     b.gwConfig.Spec.ConsulSpec.AuthSpec.Method,
		ConsulHTTPAddr:    orDefault(b.gwConfig.Spec.ConsulSpec.Address, defaultConsulAddress),
		ConsulHTTPPort:    orDefaultIntString(b.gwConfig.Spec.ConsulSpec.PortSpec.HTTP, defaultConsulHTTPPort),
		ConsulGRPCPort:    orDefaultIntString(b.gwConfig.Spec.ConsulSpec.PortSpec.GRPC, defaultConsulXDSPort),
		LogLevel:          orDefault(b.gwConfig.Spec.LogLevel, defaultLogLevel),
		GatewayHost:       "$(IP)",
		GatewayName:       b.gateway.Name,
		GatewayNamespace:  b.consulGatewayNamespace,
		PrimaryDatacenter: b.consulPrimaryDatacenter,
		SDSHost:           b.sdsHost,
		SDSPort:           b.sdsPort,
	}
	if b.requiresCA() {
		data.ConsulCAFile = consulCALocalFile
		data.ConsulCAData = b.consulCAData
	}
	if method := b.gwConfig.Spec.ConsulSpec.AuthSpec.Method; method != "" {
		data.ACLAuthMethod = method
	}
	var buf bytes.Buffer
	err := template.Must(template.New("root").
		Parse(strings.TrimSpace(gwContainerArgsTpl))).
		Execute(&buf, &data)
	if err != nil {
		return nil
	}

	return strings.Split(buf.String(), "\n")
}

func (b *GatewayDeploymentBuilder) envVars() []corev1.EnvVar {
	envVars := []corev1.EnvVar{
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
		{
			Name:  "PATH",
			Value: "/:/sbin:/bin:/usr/bin:/usr/local/bin:/bootstrap",
		},
	}

	if b.requiresCA() {
		envVars = append(envVars, corev1.EnvVar{
			Name:  api.HTTPCAFile,
			Value: consulCALocalFile,
		})
	}

	return envVars
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
				Secret: &corev1.SecretVolumeSource{
					SecretName: b.gateway.Name,
					Items: []corev1.KeyToPath{
						{
							Key:  "consul-ca-cert",
							Path: consulCAFilename,
						},
					},
					DefaultMode: nil,
					Optional:    pointer.Bool(false),
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "ca",
			MountPath: consulCALocalPath,
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
	ConsulCAFile      string
	ConsulCAData      string
	ConsulHTTPAddr    string
	ConsulHTTPPort    string
	ConsulGRPCPort    string
	ACLAuthMethod     string
	LogLevel          string
	GatewayHost       string
	GatewayName       string
	GatewayNamespace  string
	PrimaryDatacenter string
	SDSHost           string
	SDSPort           int
}

// gwContainerArgsTpl is the template for the command arguments executed in the Envoy container.
// The resulting args are split on \n to obtain a []string for the pod spec's args.
// Note: Make sure not to leave whitespace at the beginning or end of any line.
const gwContainerArgsTpl = `
-log-json
-log-level
{{ .LogLevel }}
-gateway-host
{{ .GatewayHost }}
-gateway-name
{{ .GatewayName }}
{{- if .GatewayNamespace }}
-gateway-namespace
{{ .GatewayNamespace }}
{{- end }}
-consul-http-address
{{ .ConsulHTTPAddr }}
-consul-http-port
{{ .ConsulHTTPPort }}
-consul-xds-port
{{ .ConsulGRPCPort }}
{{- if .ACLAuthMethod }}
-acl-auth-method
{{ .ACLAuthMethod }}
{{- end }}
{{- if .PrimaryDatacenter }}
-consul-primary-datacenter
{{ .PrimaryDatacenter }}
{{- end }}
-envoy-bootstrap-path
/bootstrap/envoy.json
-envoy-sds-address
{{ .SDSHost }}
-envoy-sds-port
{{ .SDSPort }}
`
