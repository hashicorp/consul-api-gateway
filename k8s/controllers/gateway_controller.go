package controllers

import (
	"context"
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
	// An optional service account to run the gateway as
	annotationServiceAccount = "polar.hashicorp.com/service-account"
	// The auth method used for consul kubernetes-based auth
	annotationServiceAuthMethod = "polar.hashicorp.com/auth-method"
	// The image to use for polar
	annotationImage = "polar.hashicorp.com/image"
	// The image to use for envoy
	annotationEnvoyImage = "polar.hashicorp.com/envoy"
	// The log-level to enable in polar
	annotationLogLevel = "polar.hashicorp.com/log-level"
	// The address to inject for initial service registration
	// if not specified, the init container will attempt to
	// use a local agent on the host on which it is running
	annotationConsulHTTPAddress = "polar.hashicorp.com/consul-http-address"
	// The scheme to use for connecting to consul
	annotationConsulScheme = "polar.hashicorp.com/consul-http-scheme"
	// The location of a secret to mount with the consul root CA information
	annotationConsulCASecret = "polar.hashicorp.com/consul-ca-secret"

	defaultEnvoyImage   = "envoyproxy/envoy:v1.18-latest"
	defaultLogLevel     = "info"
	defaultCASecret     = "consul-ca-cert"
	defaultConsulScheme = "https"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	clientruntime.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	Manager *reconciler.GatewayReconcileManager
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
		// Create a deployment for the gateway
		dep := DeploymentFor(gw)
		// Set Gateway instance as the owner and controller
		if err := ctrl.SetControllerReference(gw, dep, r.Scheme); err != nil {
			log.Error(err, "Failed to initialize gateway")
			return ctrl.Result{}, err
		}
		r.Log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
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

	// we only support a single listener for now due to service registration constraints
	if len(gw.Spec.Listeners) != 1 {
		return fmt.Errorf("invalid number of listeners '%d', only 1 supported", len(gw.Spec.Listeners))
	}
	return nil
}

// DeploymentFor returns the deployment configuration for the given gateway
func DeploymentFor(gw *gateway.Gateway) *appsv1.Deployment {
	replicas := int32(3)
	ls := labelsFor(gw)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gw.Name,
			Namespace: gw.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: podSpecFor(gw),
			},
		},
	}
}

func podSpecFor(gw *gateway.Gateway) corev1.PodSpec {
	volumes, mounts := volumesFor(gw)
	return corev1.PodSpec{
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
			Command: execCommandFor(gw),
		}},
		Volumes: volumes,
	}
}

func execCommandFor(gw *gateway.Gateway) []string {
	port := strconv.Itoa(int(gw.Spec.Listeners[0].Port))
	hostPort := fmt.Sprintf("$(IP):%s", port)
	initCommand := []string{
		"/bootstrap/polar", "exec",
		"-log-json",
		"-log-level", logLevelFor(gw),
		"-gateway-host-port", hostPort,
		"-gateway-name", gw.Name,
	}
	consulHTTPAddress := gw.Annotations[annotationConsulHTTPAddress]
	if consulHTTPAddress != "" {
		initCommand = append(initCommand, "-consul-http-address", consulHTTPAddress)
	} else {
		initCommand = append(initCommand, "-consul-http-address", "$(HOST_IP):8500")
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

func labelsFor(gw *gateway.Gateway) map[string]string {
	return map[string]string{
		"name": "polar",
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
		Complete(r)
}
