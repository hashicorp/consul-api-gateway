package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	clientruntime "sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/polar/internal/metrics"
	"github.com/hashicorp/polar/k8s/reconciler"
	"github.com/hashicorp/polar/version"
)

var (
	defaultImage string
)

func init() {
	defaultImage = fmt.Sprintf("hashicorp/polar:%s", version.Version)
}

const (
	// An optional service type to expose the gateway
	annotationServiceType = "polar.hashicorp.com/service-type"
	// If this is set to true, then the container ports are mapped
	// to host ports.
	annotationHostPortStatic = "polar.hashicorp.com/use-host-ports"
	// An optional service account to run the gateway as.
	annotationServiceAccount = "polar.hashicorp.com/service-account"
	// The image to use for polar.
	annotationImage = "polar.hashicorp.com/image"
	// The image to use for envoy.
	annotationEnvoyImage = "polar.hashicorp.com/envoy"
	// The log-level to enable in polar.
	annotationLogLevel = "polar.hashicorp.com/log-level"
	// The node selector (in JSON format) for scheduling the gateway pod.
	annotationNodeSelector = "polar.hashicorp.com/node-selector"
	// The address of the consul server to communicate with in the gateway
	// pod. If not specified, the pod will attempt to use a local agent on
	// the host on which it is running.
	annotationConsulHTTPAddress = "polar.hashicorp.com/consul-http-address"
	// The port for Consul's xDS server.
	annotationConsulXDSPort = "polar.hashicorp.com/consul-xds-port"
	// The port for Consul's HTTP server.
	annotationConsulHTTPPort = "polar.hashicorp.com/consul-http-port"
	// The scheme to use for connecting to consul.
	annotationConsulScheme = "polar.hashicorp.com/consul-http-scheme"
	// The location of a secret to mount with the consul root CA information
	annotationConsulCASecret = "polar.hashicorp.com/consul-ca-secret"
	// The auth method used for consul kubernetes-based auth.
	annotationServiceAuthMethod = "polar.hashicorp.com/consul-auth-method"

	defaultEnvoyImage     = "envoyproxy/envoy:v1.19-latest"
	defaultLogLevel       = "info"
	defaultCASecret       = "consul-ca-cert"
	defaultConsulAddress  = "$(HOST_IP)"
	defaultConsulScheme   = "https"
	defaultConsulHTTPPort = "8500"
	defaultConsulXDSPort  = "8502"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	clientruntime.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	SDSServerHost string
	SDSServerPort int
	Metrics       *metrics.K8sMetrics
	Manager       *reconciler.GatewayReconcileManager
}

//+kubebuilder:rbac:groups=polar.hashicorp.com,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=polar.hashicorp.com,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=polar.hashicorp.com,resources=gateways/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("gateway", req.NamespacedName)

	gw := &gateway.Gateway{}
	err := r.Get(ctx, req.NamespacedName, gw)
	// If the gateway object has been deleted (and we get an IsNotFound
	// error), we need to stop the associated deployment.
	if k8serrors.IsNotFound(err) {
		r.Manager.DeleteGateway(req.NamespacedName)
		// TODO stop deployment
		return ctrl.Result{}, nil
	} else if err != nil {
		r.Log.Error(err, "failed to get Gateway", "name", req.Name, "ns", req.Namespace)
		return ctrl.Result{}, err
	}

	r.Log.Info("retrieved", "name", gw.Name, "ns", gw.Namespace)
	r.Manager.UpsertGateway(gw)

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		// Validate the gateway before attempting to construct the deployment
		if err := r.Validate(ctx, gw); err != nil {
			log.Error(err, "Failed to validate gateway", "error", err)
			return ctrl.Result{}, err
		}
		// Create deployment for the gateway
		deployment := DeploymentFor(gw, r.SDSServerHost, r.SDSServerPort)
		// Create service for the gateway
		service := ServiceFor(gw)

		// Set Gateway instance as the owner and controller
		if err := ctrl.SetControllerReference(gw, deployment, r.Scheme); err != nil {
			log.Error(err, "Failed to initialize gateway deployment")
			return ctrl.Result{}, err
		}
		r.Log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.Create(ctx, deployment)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
			return ctrl.Result{}, err
		}

		if service != nil {
			// Set Service instance as the owner and controller
			if err := ctrl.SetControllerReference(gw, service, r.Scheme); err != nil {
				log.Error(err, "Failed to initialize gateway service")
				return ctrl.Result{}, err
			}
			r.Log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
			err = r.Create(ctx, service)
			if err != nil {
				log.Error(err, "Failed to create new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
				return ctrl.Result{}, err
			}
		}

		r.Metrics.NewGatewayDeployments.Inc()

		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// Validate does some basic validations on the gateway
func (r *GatewayReconciler) Validate(ctx context.Context, gw *gateway.Gateway) error {
	// check if the gateway requires a CA to inject
	if requiresCA(gw) {
		// if it does, make sure the secret exists
		if err := r.Get(ctx, namespacedCASecretFor(gw), &corev1.Secret{}); err != nil {
			return err
		}
	}

	// validate that the listeners don't conflict with names or ports
	seenPorts := make(map[gateway.PortNumber]struct{})
	seenNames := make(map[string]struct{})
	for _, listener := range gw.Spec.Listeners {
		if _, ok := seenNames[listener.Name]; ok {
			return fmt.Errorf("gateway listeners must have unique names, more than one listener has the name '%s'", listener.Name)
		}
		if _, ok := seenPorts[listener.Port]; ok {
			return fmt.Errorf("gateway listeners must bind to unique ports, more than one listener has the port '%d'", listener.Port)
		}
		seenNames[listener.Name] = struct{}{}
		seenPorts[listener.Port] = struct{}{}
	}
	return nil
}

// ServicesFor returns the service configuration for the given gateway.
// The gateway should be marked with the polar.hashicorp.com/service-type
// annotation and marked with 'ClusterIP', `NodePort` or `LoadBalancer` to
// expose the gateway listeners. Any other value does not expose the gateway.
func ServiceFor(gw *gateway.Gateway) *corev1.Service {
	serviceType := serviceTypeFor(gw)
	if serviceType == "" {
		return nil
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
			Type:     serviceType,
			Ports:    servicePortsFor(gw),
		},
	}
}

// DeploymentsFor returns the deployment configuration for the given gateway.
func DeploymentFor(gw *gateway.Gateway, sdsHost string, sdsPort int) *appsv1.Deployment {
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
				Spec: podSpecFor(gw, sdsHost, sdsPort),
			},
		},
	}
}

func podSpecFor(gw *gateway.Gateway, sdsHost string, sdsPort int) corev1.PodSpec {
	volumes, mounts := volumesFor(gw)
	return corev1.PodSpec{
		NodeSelector:       nodeSelectorFor(gw),
		ServiceAccountName: serviceAccountFor(gw),
		// the init container copies the binary into the
		// next envoy container so we can decouple the envoy
		// versions from our version of polar.
		InitContainers: []corev1.Container{{
			Image:        imageFor(gw),
			Name:         "polar-init",
			VolumeMounts: mounts,
			Command: []string{
				"cp", "/bin/polar", "/bootstrap/polar",
			},
		}},
		Containers: []corev1.Container{{
			Image:        envoyImageFor(gw),
			Name:         "polar",
			VolumeMounts: mounts,
			Ports:        containerPortsFor(gw),
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
			Command: execCommandFor(gw, sdsHost, sdsPort),
		}},
		Volumes: volumes,
	}
}

func execCommandFor(gw *gateway.Gateway, sdsHost string, sdsPort int) []string {
	initCommand := []string{
		"/bootstrap/polar", "exec",
		"-log-json",
		"-log-level", logLevelFor(gw),
		"-gateway-host", "$(IP)",
		"-gateway-name", gw.Name,
		"-consul-http-address", consulAddressFor(gw),
		"-consul-http-port", httpPortFor(gw),
		"-consul-xds-port", xdsPortFor(gw),
		"-envoy-bootstrap-path", "/bootstrap/envoy.json",
		"-envoy-sds-address", sdsHost,
		"-envoy-sds-port", strconv.Itoa(sdsPort),
	}

	authMethod := gw.Annotations[annotationServiceAuthMethod]
	if authMethod != "" {
		initCommand = append(initCommand, "-acl-auth-method", authMethod)
	}

	if requiresCA(gw) {
		initCommand = append(initCommand, "-consul-ca-cert-file", "/ca/tls.crt")
	}
	return initCommand
}

func volumesFor(gw *gateway.Gateway) ([]corev1.Volume, []corev1.VolumeMount) {
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
	if requiresCA(gw) {
		caCertSecret := caSecretFor(gw)
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

func imageFor(gw *gateway.Gateway) string {
	image := gw.Annotations[annotationImage]
	if image == "" {
		return defaultImage
	}
	return image
}

func envoyImageFor(gw *gateway.Gateway) string {
	image := gw.Annotations[annotationEnvoyImage]
	if image == "" {
		return defaultEnvoyImage
	}
	return image
}

func serviceAccountFor(gw *gateway.Gateway) string {
	return gw.Annotations[annotationServiceAccount]
}

func consulSchemeFor(gw *gateway.Gateway) string {
	if gw.Annotations[annotationConsulScheme] != "http" {
		return "https"
	}
	return "http"
}

func namespacedCASecretFor(gw *gateway.Gateway) clientruntime.ObjectKey {
	name := gw.Annotations[annotationConsulCASecret]
	if name == "" {
		name = defaultCASecret
	}
	return clientruntime.ObjectKey{
		Namespace: gw.Namespace,
		Name:      name,
	}
}

func caSecretFor(gw *gateway.Gateway) string {
	name := gw.Annotations[annotationConsulCASecret]
	if name == "" {
		return defaultCASecret
	}
	return name
}

func logLevelFor(gw *gateway.Gateway) string {
	logLevel := gw.Annotations[annotationLogLevel]
	if logLevel == "" {
		return defaultLogLevel
	}
	return logLevel
}

func serviceTypeFor(gw *gateway.Gateway) corev1.ServiceType {
	switch serviceType := corev1.ServiceType(gw.Annotations[annotationServiceType]); serviceType {
	case corev1.ServiceTypeClusterIP:
		fallthrough
	case corev1.ServiceTypeNodePort:
		fallthrough
	case corev1.ServiceTypeLoadBalancer:
		return serviceType
	default:
		return ""
	}
}

func servicePortsFor(gw *gateway.Gateway) []corev1.ServicePort {
	ports := []corev1.ServicePort{}
	for _, listener := range gw.Spec.Listeners {
		ports = append(ports, corev1.ServicePort{
			Name:     listener.Name,
			Protocol: "TCP",
			Port:     int32(listener.Port),
		})
	}
	return ports
}

func containerPortsFor(gw *gateway.Gateway) []corev1.ContainerPort {
	useStaticMapping := hostPortIsStatic(gw)
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
		if useStaticMapping {
			port.HostPort = int32(listener.Port)
		}
		ports = append(ports, port)
	}
	return ports
}

func consulAddressFor(gw *gateway.Gateway) string {
	consulAddress := gw.Annotations[annotationConsulHTTPAddress]
	if consulAddress == "" {
		return defaultConsulAddress
	}
	return consulAddress
}

func xdsPortFor(gw *gateway.Gateway) string {
	port := gw.Annotations[annotationConsulXDSPort]
	_, err := strconv.Atoi(port)
	if err != nil || port == "" {
		// if we encounter an error, just ignore the annotation
		return defaultConsulXDSPort
	}
	return port
}

func httpPortFor(gw *gateway.Gateway) string {
	port := gw.Annotations[annotationConsulHTTPPort]
	_, err := strconv.Atoi(port)
	if err != nil || port == "" {
		// if we encounter an error, just ignore the annotation
		return defaultConsulHTTPPort
	}
	return port
}

func hostPortIsStatic(gw *gateway.Gateway) bool {
	return gw.Annotations[annotationHostPortStatic] == "true"
}

func nodeSelectorFor(gw *gateway.Gateway) map[string]string {
	selector := make(map[string]string)
	nodeSelector := gw.Annotations[annotationNodeSelector]
	if nodeSelector == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(nodeSelector), &selector); err != nil {
		// if we encounter an error, just ignore the annotation
		return nil
	}
	return selector
}

func labelsFor(gw *gateway.Gateway) map[string]string {
	return map[string]string{
		"name":      "polar-" + gw.Name,
		"managedBy": "polar",
	}
}

func requiresCA(gw *gateway.Gateway) bool {
	return consulSchemeFor(gw) == "https"
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For()
		For(&gateway.Gateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
