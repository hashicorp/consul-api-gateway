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
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/polar/k8s/reconciler"
)

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
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Log          logr.Logger
	Scheme       *runtime.Scheme
	CACertSecret string

	image string

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
		// Define a new deployment
		if err := r.validateGateway(gw); err != nil {
			log.Error(err, "Failed to validate gateway", "error", err)
			return ctrl.Result{}, err
		}
		dep, err := r.deploymentForGateway(gw)
		if err != nil {
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

func (r *GatewayReconciler) validateGateway(gw *gateway.Gateway) error {
	// we only support a single listener for now due to service registration constraints
	if len(gw.Spec.Listeners) != 1 {
		return fmt.Errorf("invalid number of listeners '%d', only 1 supported", len(gw.Spec.Listeners))
	}
	return nil
}

func (r *GatewayReconciler) deploymentForGateway(gw *gateway.Gateway) (*appsv1.Deployment, error) {
	replicas := int32(3)
	ls := labelsForGateway(gw)
	dep := &appsv1.Deployment{
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
				Spec: podSpecForGateway(gw, r.CACertSecret, r.image),
			},
		},
	}

	// Set Gateway instance as the owner and controller
	if err := ctrl.SetControllerReference(gw, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}

func podSpecForGateway(gw *gateway.Gateway, caCertSecret, defaultImage string) corev1.PodSpec {
	image := imageForGateway(gw, defaultImage)
	cmd := execCommandForGateway(gw, caCertSecret)
	volumes, mounts := volumesForGateway(gw, caCertSecret)
	serviceAccount := serviceAccountForGateway(gw)
	return corev1.PodSpec{
		ServiceAccountName: serviceAccount,
		Containers: []corev1.Container{{
			Image:        image,
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
			Command: cmd,
		}},
		Volumes: volumes,
	}
}

func execCommandForGateway(gw *gateway.Gateway, caCertSecret string) []string {
	port := strconv.Itoa(int(gw.Spec.Listeners[0].Port))
	hostPort := fmt.Sprintf("$(IP):%s", port)
	initCommand := []string{
		"polar", "exec",
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
	if caCertSecret != "" {
		initCommand = append(initCommand, "-consul-ca-cert-file", "/ca/tls.crt")
	}
	return initCommand
}

func volumesForGateway(gw *gateway.Gateway, caCertSecret string) ([]corev1.Volume, []corev1.VolumeMount) {
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
		MountPath: "/polar",
	}, {
		Name:      "certs",
		MountPath: "/certs",
	}}
	if caCertSecret != "" {
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

func imageForGateway(gw *gateway.Gateway, defaultImage string) string {
	image := gw.Annotations[annotationImage]
	if image == "" {
		return defaultImage
	}
	return image
}

func serviceAccountForGateway(gw *gateway.Gateway) string {
	return gw.Annotations[annotationServiceAccount]
}

func labelsForGateway(gw *gateway.Gateway) map[string]string {
	return map[string]string{
		"name": "polar",
	}
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
