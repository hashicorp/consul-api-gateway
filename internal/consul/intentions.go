package consul

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

//go:generate mockgen -source ./intentions.go -destination ./mocks/intentions.go -package mocks consulDiscoveryChains,consulConfigEntries

const (
	updateIntentionsMaxRetries = 3
)

var (
	intentionSyncInterval         = 60 * time.Second
	updateIntentionsRetryInterval = time.Second
)

type consulDiscoveryChains interface {
	Get(name string, opts *api.DiscoveryChainOptions, qopts *api.QueryOptions) (*api.DiscoveryChainResponse, *api.QueryMeta, error)
}

type consulConfigEntries interface {
	CAS(entry api.ConfigEntry, index uint64, w *api.WriteOptions) (bool, *api.WriteMeta, error)
	Delete(kind string, name string, w *api.WriteOptions) (*api.WriteMeta, error)
	Get(kind string, name string, q *api.QueryOptions) (api.ConfigEntry, *api.QueryMeta, error)
}

// IntentionsReconciler maintains a reconcile loop that computes the changes required to the intention graph to allow
// traffic from the api gateway to target services. Changes are detected by watching the service's computed discovery
// chain and iterating through the included targets.
type IntentionsReconciler struct {
	consulDisco    consulDiscoveryChains
	consulConfig   consulConfigEntries
	serviceName    core.GatewayID
	discoChainChan <-chan *api.CompiledDiscoveryChain
	ctx            context.Context
	stop           context.CancelFunc

	targetIndex      *common.ServiceNameIndex
	targetTombstones *common.ServiceNameIndex

	logger hclog.Logger
}

func NewIntentionsReconciler(consul *api.Client, id core.GatewayID, logger hclog.Logger) *IntentionsReconciler {
	r := newIntentionsReconciler(consul.DiscoveryChain(), consul.ConfigEntries(), id, logger)
	r.discoChainChan = r.watchDiscoveryChain()
	go r.reconcileLoop()
	return r
}

func newIntentionsReconciler(disco consulDiscoveryChains, config consulConfigEntries, id core.GatewayID, logger hclog.Logger) *IntentionsReconciler {
	ctx, cancel := context.WithCancel(context.Background())
	return &IntentionsReconciler{
		consulDisco:      disco,
		consulConfig:     config,
		serviceName:      id,
		targetIndex:      common.NewServiceNameIndex(),
		targetTombstones: common.NewServiceNameIndex(),
		ctx:              ctx,
		stop:             cancel,
		logger:           logger,
	}
}

func (r *IntentionsReconciler) Stop() {
	r.stop()
}

// sourceIntention builds the api gateway source rule for updating intentions
func (r *IntentionsReconciler) sourceIntention() *api.SourceIntention {
	return &api.SourceIntention{
		Name:        r.serviceName.Service,
		Namespace:   r.serviceName.ConsulNamespace,
		Action:      api.IntentionActionAllow,
		Description: fmt.Sprintf("Allow traffic from Consul API Gateway. reconciled by controller at %s", time.Now().Format(time.RFC3339)),
	}
}

// reconcileLoop runs until the struct context is cancelled, handling of the discovery chain is fired under 2 conditions.
// If the background blocking query completes, the chain is sent over the discoChainChan and is handled by the loop.
// A ticker fires every 60s to do sync of any intentions that failed to sync during an update
func (r *IntentionsReconciler) reconcileLoop() {
	timer := time.NewTicker(intentionSyncInterval)
	for {
		select {
		case <-r.ctx.Done():
			return
		case chain := <-r.discoChainChan:
			r.handleChain(chain)
		case <-timer.C:
			r.syncIntentions()
		}
	}
}

func (r *IntentionsReconciler) syncIntentions() {
	for _, target := range r.targetIndex.All() {
		if err := r.updateIntentionSources(target, r.sourceIntention(), nil); err != nil {
			r.logger.Error("failed to update intention with added gateway source", "name", target.Name, "namespace", target.Namespace, "error", err)
		}
	}

	for _, target := range r.targetTombstones.All() {
		if err := r.updateIntentionSources(target, nil, r.sourceIntention()); err != nil {
			r.logger.Error("failed to update intention with added gateway source", "name", target.Name, "namespace", target.Namespace, "error", err)
			continue
		}
		r.targetTombstones.Remove(target)
	}
}

// handleChain computes the added and removed targets from the last change and applies intention changes
func (r *IntentionsReconciler) handleChain(chain *api.CompiledDiscoveryChain) {
	newTargetIndex := common.NewServiceNameIndex()
	for _, target := range chain.Targets {
		newTargetIndex.Add(api.CompoundServiceName{Name: target.Name, Namespace: target.Namespace})
	}

	added, removed := r.targetIndex.Diff(newTargetIndex)

	for _, target := range added {
		if err := r.updateIntentionSources(target, r.sourceIntention(), nil); err != nil {
			r.logger.Error("failed to update intention with added gateway source", "name", target.Name, "namespace", target.Namespace, "error", err)
		}
		// should no longer be in tombstones
		r.targetTombstones.Remove(target)
	}

	for _, target := range removed {
		if err := r.updateIntentionSources(target, nil, r.sourceIntention()); err != nil {
			r.logger.Error("failed to update intention with added gateway source", "name", target.Name, "namespace", target.Namespace, "error", err)
			r.targetTombstones.Add(target)
		}
	}

	r.targetIndex = newTargetIndex
}

// watchDiscoveryChain uses blocking queries to poll for changes in the services discovery chain
func (r *IntentionsReconciler) watchDiscoveryChain() <-chan *api.CompiledDiscoveryChain {
	results := make(chan *api.CompiledDiscoveryChain)
	go func() {
		defer close(results)
		var index uint64
		for {
			opts := &api.QueryOptions{WaitIndex: index, Namespace: r.serviceName.ConsulNamespace}
			resp, meta, err := r.consulDisco.Get(r.serviceName.Service, nil, opts.WithContext(r.ctx))
			if err != nil {
				r.logger.Warn("blocking query for gateway discovery chain failed", "error", err)
				select {
				case <-r.ctx.Done():
					return
				case <-time.After(time.Second):
					// avoid hot looping on error
				}
				continue
			}

			if meta.LastIndex < index {
				index = 0
			} else {
				index = meta.LastIndex
			}

			select {
			case <-r.ctx.Done():
				return
			case results <- resp.Chain:
			}
		}
	}()
	return results
}

func (r *IntentionsReconciler) updateIntentionSources(name api.CompoundServiceName, toAdd, toRemove *api.SourceIntention) error {
	if toAdd == nil && toRemove == nil {
		return nil
	}
	return backoff.Retry(func() error {
		intention, idx, err := r.getOrInitIntention(name)
		if err != nil {
			return err
		}

		// changed tracks if any modifications have been made to the intentionnn
		var changed bool

		if toAdd != nil {
			// check if source is already in intention
			var found bool
			for _, src := range intention.Sources {
				if src.Name == toAdd.Name && src.Namespace == toAdd.Namespace {
					found = true
					break
				}
			}

			// add source to intention
			if !found {
				intention.Sources = append(intention.Sources, toAdd)
				changed = true
			}
		}

		if toRemove != nil {
			// find and remove source with matching name and namespace
			for i, src := range intention.Sources {
				if src.Name == toRemove.Name && src.Namespace == toRemove.Namespace {
					intention.Sources = append(intention.Sources[:i], intention.Sources[i+1:]...)
					changed = true
					break
				}
			}
		}

		// if no intention changes stop here
		if !changed {
			return nil
		}

		// if the intention now has no sources it can be deleted
		if len(intention.Sources) == 0 {
			_, err := r.consulConfig.Delete(api.ServiceIntentions, intention.Name, &api.WriteOptions{Namespace: intention.Namespace})
			if err != nil {
				return err
			}
			return nil
		}

		// update the intention through CAS
		ok, _, err := r.consulConfig.CAS(intention, idx, nil)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("CAS operation failed")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(updateIntentionsRetryInterval), updateIntentionsMaxRetries))
}

func (r *IntentionsReconciler) getOrInitIntention(name api.CompoundServiceName) (intention *api.ServiceIntentionsConfigEntry, idx uint64, err error) {
	entry, meta, err := r.consulConfig.Get(api.ServiceIntentions, name.Name, &api.QueryOptions{Namespace: name.Namespace})
	if err == nil {
		intention = entry.(*api.ServiceIntentionsConfigEntry)
		return intention, meta.LastIndex, nil
	}

	if strings.Contains(err.Error(), "Unexpected response code: 404") {
		return &api.ServiceIntentionsConfigEntry{
			Kind:      api.ServiceIntentions,
			Name:      name.Name,
			Namespace: name.Namespace,
		}, 0, nil
	}

	return nil, 0, err
}
