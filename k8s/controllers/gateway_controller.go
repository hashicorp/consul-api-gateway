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

	"github.com/hashicorp/polar/k8s/consul"
	"github.com/hashicorp/polar/k8s/reconciler"
)

const (
	// An optional service account to run the gateway as
	annotationServiceAccount = "polar.hashicorp.com/service-account"
	// The auth method used for consul kubernetes-based auth
	annotationServiceAuthMethod = "polar.hashicorp.com/auth-method"
	// The image to use for polar
	annotationImage = "polar.hashicorp.com/image"
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

	Manager       *reconciler.GatewayReconcileManager
	CertGenerator *consul.CertGenerator
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
		dep, err := r.deploymentForGateway(ctx, gw)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "error", err)
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

func (r *GatewayReconciler) deploymentForGateway(ctx context.Context, gw *gateway.Gateway) (*appsv1.Deployment, error) {
	replicas := int32(3)
	ls := labelsForGateway(gw)

	// we only support a single listener for now due to service registration constraints
	if len(gw.Spec.Listeners) != 1 {
		return nil, fmt.Errorf("invalid number of listeners '%d', only 1 supported", len(gw.Spec.Listeners))
	}

	cert, err := r.CertGenerator.GenerateFor(gw.Name)
	if err != nil {
		return nil, err
	}

	certName := fmt.Sprintf("%s-cert", gw.Name)

	// do we need to GC this?
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certName,
			Namespace: gw.Namespace,
		},
		StringData: map[string]string{
			"root-ca.pem":     cert.Root.RootCertPEM,
			"client-key.pem":  cert.Client.PrivateKeyPEM,
			"client-cert.pem": cert.Client.PrivateKeyPEM,
		},
	}

	ctrl.SetControllerReference(gw, secret, r.Scheme)

	if err := r.Create(ctx, secret); err != nil {
		return nil, err
	}

	image := gw.Annotations[annotationImage]
	if image == "" {
		image = r.image
	}

	volumes := []corev1.Volume{{
		Name: "bootstrap",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}, {
		Name: "certs",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: certName,
			},
		},
	}}

	mounts := []corev1.VolumeMount{{
		Name:      "bootstrap",
		MountPath: "/polar",
	}, {
		Name:      "certs",
		MountPath: "/certs",
		ReadOnly:  true,
	}}

	cmd := initCommandForGateway(gw)

	if r.CACertSecret != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.CACertSecret,
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "ca",
			MountPath: "/ca",
			ReadOnly:  true,
		})
		cmd = append(cmd, "-consul-ca-cert-file", "/ca/tls.crt")
	}

	port := strconv.Itoa(int(gw.Spec.Listeners[0].Port))
	listener := fmt.Sprintf("while true; do printf 'HTTP/1.1 200 OK\n\nOK' | nc -l %s; done", port)
	podSpec := corev1.PodSpec{
		ServiceAccountName: "polar",
		InitContainers: []corev1.Container{{
			Image:        image,
			Name:         "polar-init",
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
		Containers: []corev1.Container{{
			Image: image,
			Name:  "polar",
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "bootstrap",
				MountPath: "/polar",
			}, {
				Name:      "certs",
				MountPath: "/certs",
				ReadOnly:  true,
			}},
			Command: []string{
				"/bin/sh", "-c", listener,
			},
			Lifecycle: &corev1.Lifecycle{
				PreStop: &corev1.Handler{
					Exec: &corev1.ExecAction{
						Command: append(initCommandForGateway(gw), "-deregister"),
					},
				},
			},
		}},
		Volumes: volumes,
	}

	serviceAccount := gw.Annotations[annotationServiceAccount]
	if serviceAccount != "" {
		podSpec.ServiceAccountName = serviceAccount
	}

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
				Spec: podSpec,
			},
		},
	}

	// Set Gateway instance as the owner and controller
	ctrl.SetControllerReference(gw, dep, r.Scheme)
	return dep, nil
}

func initCommandForGateway(gw *gateway.Gateway) []string {
	port := strconv.Itoa(int(gw.Spec.Listeners[0].Port))

	initCommand := []string{
		"polar", "init",
		"-gateway-ip", "$(IP)",
		"-gateway-port", port,
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
	return initCommand
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
		Owns(&corev1.Secret{}).
		Complete(r)
}
