package reconciler

import (
	"context"
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/k8s/consul"
	"github.com/hashicorp/polar/k8s/object"
	"github.com/hashicorp/polar/k8s/routes"
	"github.com/hashicorp/polar/k8s/utils"

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	invalidRouteRefReason = "InvalidRouteRef"
	routeAdmittedReason   = "RouteAdmitted"
)

type GatewayReconciler struct {
	controllerName    string
	ctx               context.Context
	signalReconcileCh chan struct{}
	stopReconcileCh   chan struct{}

	consul *consul.ConfigEntriesReconciler

	kubeGateway *gw.Gateway
	kubeRoutes  *routes.KubernetesRoutes
	status      *object.StatusWorker

	logger hclog.Logger
}

type gatewayReconcilerArgs struct {
	controllerName string
	consul         *api.Client
	gateway        *gw.Gateway
	routes         *routes.KubernetesRoutes
	status         *object.StatusWorker
	logger         hclog.Logger
}

func newReconcilerForGateway(ctx context.Context, args *gatewayReconcilerArgs) *GatewayReconciler {
	logger := args.logger.With("gateway", args.gateway.Name, "namespace", args.gateway.Namespace)
	return &GatewayReconciler{
		controllerName:    args.controllerName,
		ctx:               ctx,
		signalReconcileCh: make(chan struct{}, 1), // buffered chan allow for a single pending reconcile signal
		stopReconcileCh:   make(chan struct{}, 0),
		consul:            consul.NewReconciler(args.consul, logger),
		kubeGateway:       args.gateway,
		kubeRoutes:        args.routes,
		status:            args.status,

		logger: logger,
	}
}

func (c *GatewayReconciler) signalReconcile() {
	select {
	case c.signalReconcileCh <- struct{}{}:
	default:
	}
}

func (c *GatewayReconciler) setInitialGatewayStatus() {
	obj := object.New(c.kubeGateway)
	obj.Status.Mutate(func(status interface{}) interface{} {
		gwStatus := status.(*gw.GatewayStatus)
		gwStatus.Conditions = []metav1.Condition{
			{
				Type:               string(gw.GatewayConditionReady),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: obj.GetGeneration(),
				LastTransitionTime: metav1.Now(),
				Reason:             string(gw.GatewayReasonListenersNotReady),
				Message:            "waiting for reconcile",
			},
			{
				Type:               string(gw.GatewayConditionScheduled),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: obj.GetGeneration(),
				LastTransitionTime: metav1.Now(),
				Reason:             string(gw.GatewayReasonNotReconciled),
				Message:            "waiting for reconcile",
			},
		}
		gwStatus.Listeners = make([]gw.ListenerStatus, len(c.kubeGateway.Spec.Listeners))
		for idx, listener := range c.kubeGateway.Spec.Listeners {
			gwStatus.Listeners[idx] = gw.ListenerStatus{
				Name:           listener.Name,
				AttachedRoutes: 0,
				Conditions: []metav1.Condition{
					{
						Type:               string(gw.ListenerConditionReady),
						Status:             metav1.ConditionFalse,
						ObservedGeneration: obj.GetGeneration(),
						LastTransitionTime: metav1.Now(),
						Reason:             string(gw.ListenerReasonPending),
						Message:            "pending reconcile",
					},
				},
			}
		}
		return gwStatus
	})
	c.status.Push(obj)
}

func (c *GatewayReconciler) loop() {
	c.setInitialGatewayStatus()
	for {
		select {
		case <-c.signalReconcileCh:
			// make sure theres no pending stop signal before starting a reconcile
			// this can happen if both chans are sending as selection of cases is non deterministic
			select {
			case <-c.stopReconcileCh:
				return
			default:
				if err := c.reconcile(); err != nil {
					c.logger.Error("failed to reconcile gateway", "error", err)
				}
			}
		case <-c.ctx.Done():
			return
		case <-c.stopReconcileCh:
			return
		}
	}
}

func (c *GatewayReconciler) stop() {
	c.stopReconcileCh <- struct{}{}
}

// reconcile should never be called outside of loop() to ensure it is not invoked concurrently
func (c *GatewayReconciler) reconcile() error {
	if c.logger.IsTrace() {
		start := time.Now()
		c.logger.Trace("reconcile started")
		defer c.logger.Trace("reconcile finished", "duration", hclog.Fmt("%dms", time.Now().Sub(start).Milliseconds()))
	}
	gatewayName := utils.KubeObjectNamespacedName(c.kubeGateway)
	resolvedGateway := consul.NewResolvedGateway(gatewayName)

	for _, kubeRoute := range c.kubeRoutes.HTTPRoutes() {
		status := newRouteStatusBuilder(kubeRoute)
		// route has one or more references to gateway, each listener must admit the route
		// if the route references a specific listener, then it needs to be checked against the listener name
		for _, ref := range kubeRoute.CommonRouteSpec().ParentRefs {
			for _, kubeListener := range c.kubeGateway.Spec.Listeners {
				if err := kubeRoute.ParentRefAllowed(ref, gatewayName, kubeListener); err != nil {
					status.addRef(ref, false, invalidRouteRefReason, err.Error())
					continue
				}

				admit, reason, message := kubeRoute.IsAdmittedByGatewayListener(gatewayName, kubeListener.Routes)
				status.addRef(ref, admit, reason, message)
				if admit {
					resolvedGateway.AddRoute(kubeListener, kubeRoute)
				}
			}
		}
		kubeRoute.Status.Mutate(func(s interface{}) interface{} {
			r, _ := s.(*gw.HTTPRouteStatus)
			r.Parents = status.build(c.controllerName, r.Parents)
			return r
		})
		if kubeRoute.Status.IsDirty() {
			c.status.Push(kubeRoute.Object)
		}
	}
	return c.consul.ReconcileGateway(resolvedGateway)
}

type routeStatusBuilder struct {
	route object.KubeObj
	refs  map[gw.ParentRef]routeStatus
}

type routeStatus struct {
	admitted bool
	reason   string
	message  string
}

func newRouteStatusBuilder(route object.KubeObj) *routeStatusBuilder {
	return &routeStatusBuilder{
		route: route,
		refs:  map[gw.ParentRef]routeStatus{},
	}
}

func (b *routeStatusBuilder) addRef(ref gw.ParentRef, admitted bool, reason, message string) {
	if status, ok := b.refs[ref]; !ok || (!status.admitted && admitted) {
		b.refs[ref] = routeStatus{
			admitted: admitted,
			reason:   reason,
			message:  message,
		}
	}
}

func (b *routeStatusBuilder) build(controller string, current []gw.RouteParentStatus) []gw.RouteParentStatus {
	result := make([]gw.RouteParentStatus, 0, len(b.refs))

	// first add any existing Status that aren't managed by this controller
	for _, status := range current {
		if status.Controller != controller {
			result = append(result, status)
		}
	}

	for ref, status := range b.refs {
		condition := metav1.Condition{
			Type:               string(gw.ConditionRouteAdmitted),
			ObservedGeneration: b.route.GetGeneration(),
			LastTransitionTime: metav1.Now(),
		}
		if status.admitted {
			condition.Status = metav1.ConditionTrue
			condition.Reason = routeAdmittedReason
			condition.Message = "Route allowed"
		} else {
			condition.Status = metav1.ConditionFalse
			condition.Reason = status.reason
			condition.Message = status.message
		}
		result = append(result, gw.RouteParentStatus{
			ParentRef:  ref,
			Controller: controller,
			Conditions: []metav1.Condition{condition},
		})
	}

	// sort so the results are deterministic
	sort.SliceStable(result, func(i, j int) bool {
		return parentRefString(result[i].ParentRef) > parentRefString(result[j].ParentRef)
	})

	return result
}

func drefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parentRefString(ref gw.ParentRef) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", drefString(ref.Group), drefString(ref.Kind), drefString(ref.Namespace), ref.Name, drefString(ref.SectionName))
}
