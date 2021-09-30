package reconciler

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/go-hclog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type K8sGatewayClasses struct {
	logger hclog.Logger
	client gatewayclient.Client

	gatewayClasses map[string]*gw.GatewayClass
	mutex          sync.RWMutex
}

func NewK8sGatewayClasses(logger hclog.Logger, client gatewayclient.Client) *K8sGatewayClasses {
	return &K8sGatewayClasses{
		logger:         logger,
		client:         client,
		gatewayClasses: make(map[string]*gw.GatewayClass),
	}
}

func (g *K8sGatewayClasses) Exists(name string) bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	_, found := g.gatewayClasses[name]
	return found
}

func (g *K8sGatewayClasses) Upsert(ctx context.Context, gc *gw.GatewayClass, validParameters bool) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	var currentGen int64
	if current, ok := g.gatewayClasses[gc.Name]; ok {
		currentGen = current.GetGeneration()
	}
	if gc.Generation > currentGen {
		g.gatewayClasses[gc.Name] = gc

		conditions := gatewayClassConditions(gc, validParameters)
		if utils.IsFieldUpdated(gc.Status.Conditions, conditions) {
			gc.Status.Conditions = conditions
			if err := g.client.UpdateStatus(ctx, gc); err != nil {
				g.logger.Error("error updating gatewayclass status", "error", err)
				return err
			}
		}
	}

	return nil
}

func (g *K8sGatewayClasses) Delete(name string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	delete(g.gatewayClasses, name)
}

func gatewayClassConditions(gc *gw.GatewayClass, validParameters bool) []metav1.Condition {
	if validParameters {
		return []metav1.Condition{
			{
				Type:               string(gw.GatewayClassConditionStatusAdmitted),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: gc.Generation,
				LastTransitionTime: metav1.Now(),
				Reason:             string(gw.GatewayClassReasonAdmitted),
				Message:            fmt.Sprintf("admitted by controller %q", gc.Spec.Controller),
			},
		}
	}
	return []metav1.Condition{
		{
			Type:               string(gw.GatewayClassConditionStatusAdmitted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gc.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gw.GatewayClassReasonInvalidParameters),
			Message:            fmt.Sprintf("rejected by controller %q", gc.Spec.Controller),
		},
	}
}
