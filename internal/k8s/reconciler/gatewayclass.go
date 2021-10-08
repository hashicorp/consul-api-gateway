package reconciler

import (
	"context"
	"errors"
	"sync"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	apigwv1alpha1 "github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
	"github.com/hashicorp/go-hclog"
	"k8s.io/apimachinery/pkg/types"
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

func (g *K8sGatewayClasses) Upsert(ctx context.Context, gc *gw.GatewayClass) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if current, ok := g.gatewayClasses[gc.Name]; ok {
		if current.Generation > gc.Generation {
			// we have an old gatewayclass update ignore
			return nil
		}
	}

	g.gatewayClasses[gc.Name] = gc

	status, err := g.Validate(ctx, gc)
	if err != nil {
		g.logger.Error("error validating gatewayclass", "error", err)
		return err
	}
	conditions := status.Conditions(gc.Generation)
	if !conditionsEqual(conditions, gc.Status.Conditions) {
		gc.Status.Conditions = conditions
		if err := g.client.UpdateStatus(ctx, gc); err != nil {
			g.logger.Error("error updating gatewayclass status", "error", err)
			return err
		}
	}

	return nil
}

func (g *K8sGatewayClasses) Validate(ctx context.Context, gc *gw.GatewayClass) (GatewayClassStatus, error) {
	status := GatewayClassStatus{}
	// only validate if we actually have a config reference
	if parametersRef := gc.Spec.ParametersRef; parametersRef != nil {
		// check that we're using a typed config
		if parametersRef.Group != apigwv1alpha1.Group || parametersRef.Kind != apigwv1alpha1.GatewayClassConfigKind {
			status.Accepted.InvalidParameters = errors.New("unsupported gateway class configuration")
			return status, nil
		}

		// ignore namespace since we're cluster-scoped
		found, err := g.client.GetGatewayClassConfig(ctx, types.NamespacedName{Name: parametersRef.Name})
		if err != nil {
			return status, err
		}
		if found == nil {
			status.Accepted.InvalidParameters = errors.New("gateway class not found")
			return status, nil
		}
	}

	return status, nil
}

func (g *K8sGatewayClasses) Delete(name string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	delete(g.gatewayClasses, name)
}
