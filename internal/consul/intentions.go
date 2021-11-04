package consul

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

//go:generate mockgen -source ./intentions.go -destination ./mocks/intentions.go -package mocks consulDiscoveryChains,consulConfigEntries

const (
	updateIntentionsMaxRetries = 3
)

var (
	// intentionSyncInterval is the time between periodic intention syncs, intention syncs may happen more often as
	// a result of discovery chain updates for example.
	intentionSyncInterval = 60 * time.Second
	// updateIntentionsRetryInterval is the interval between attempts to update intention config entries if the update fails
	updateIntentionsRetryInterval = time.Second
)

// consulDiscoveryChains matches the Consul api DiscoveryChain client
type consulDiscoveryChains interface {
	Get(name string, opts *api.DiscoveryChainOptions, qopts *api.QueryOptions) (*api.DiscoveryChainResponse, *api.QueryMeta, error)
}

// consulConfigEntries matches the Consul api ConfigEntries client
type consulConfigEntries interface {
	CAS(entry api.ConfigEntry, index uint64, w *api.WriteOptions) (bool, *api.WriteMeta, error)
	Delete(kind string, name string, w *api.WriteOptions) (*api.WriteMeta, error)
	Get(kind string, name string, q *api.QueryOptions) (api.ConfigEntry, *api.QueryMeta, error)
}

// IntentionsReconciler maintains a reconcile loop that computes the changes required to the intention graph to allow
// traffic from the api gateway to target services. Changes are detected by watching the service's computed discovery
// chain and iterating through the included targets.
type IntentionsReconciler struct {
	// consulDisco and consulConfig are the Consul client interfaces, mocked out for testing
	consulDisco  consulDiscoveryChains
	consulConfig consulConfigEntries
	// service name of the ingress gateway
	gatewayName api.CompoundServiceName

	// chainWatchersMutex syncs calls to chainWatchers map
	chainWatchersMutex sync.Mutex
	// each api.IngressService of the ingress gateway must have their discovery chain watched to add an intention
	// to each target. chainWatchers is a map of api.IngressService name to a struct that handles watching of the
	// service's compile discovery chain
	chainWatchers map[api.CompoundServiceName]*discoChainWatcher
	// results from each discoChainWatcher is sent to discoChainChan
	discoChainChan chan *discoChainWatchResult
	// ingressServiceIndex is the flattened list of service names of all api.IngressService in the ingress gateway
	ingressServiceIndex *common.ServiceNameIndex

	// ctx is the parent context passed to watchers and is used in the main reconcile loop
	ctx context.Context
	// stop can be called the cancelled ctx and stop the main reconcile loop
	stop context.CancelFunc

	// forceSyncChan is used to trigger a blocking intention sync. An error chan is sent and can be blocked on for
	// the result to the intention sync.
	forceSyncChan chan (chan error)

	// targetIndex tracks the targets that need to have intentions added and the references to them.
	// if a target is removed during reconciliation it is only added to targetTombstones if no other
	// IngresService's discovery chain referenced it.
	targetIndex *intentionTargetReferenceIndex
	// targetTombstones is the set of services which need to have an intention source removed
	targetTombstones *common.ServiceNameIndex

	logger hclog.Logger
}

func NewIntentionsReconciler(consul *api.Client, ingress *api.IngressGatewayConfigEntry, logger hclog.Logger) *IntentionsReconciler {
	name := api.CompoundServiceName{Name: ingress.Name, Namespace: ingress.Namespace}
	r := newIntentionsReconciler(consul.DiscoveryChain(), consul.ConfigEntries(), name, logger)
	r.updateChainWatchers(ingress)
	go r.reconcileLoop()
	return r
}

func newIntentionsReconciler(disco consulDiscoveryChains, config consulConfigEntries, name api.CompoundServiceName, logger hclog.Logger) *IntentionsReconciler {
	ctx, cancel := context.WithCancel(context.Background())
	return &IntentionsReconciler{
		consulDisco:         disco,
		consulConfig:        config,
		gatewayName:         name,
		chainWatchers:       map[api.CompoundServiceName]*discoChainWatcher{},
		discoChainChan:      make(chan *discoChainWatchResult),
		ingressServiceIndex: common.NewServiceNameIndex(),
		targetIndex:         &intentionTargetReferenceIndex{refs: map[api.CompoundServiceName]*common.ServiceNameIndex{}},
		targetTombstones:    common.NewServiceNameIndex(),
		ctx:                 ctx,
		stop:                cancel,
		forceSyncChan:       make(chan (chan error)),
		logger:              logger,
	}
}

func (r *IntentionsReconciler) Stop() {
	r.stop()
}

func (r *IntentionsReconciler) SetIngressServices(igw *api.IngressGatewayConfigEntry) {
	r.updateChainWatchers(igw)
}

// Reconcile forces a synchronous reconcile, returning any errors that occurred as a result
func (r *IntentionsReconciler) Reconcile() error {
	// create an error channel to return the result from intention synchronization
	errCh := make(chan error)
	defer close(errCh)
	select {
	case r.forceSyncChan <- errCh:
	case <-r.ctx.Done():
		return r.ctx.Err()
	}

	// wait for synchronization to complete and return the result
	select {
	case err := <-errCh:
		return err
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

// sourceIntention builds the api gateway source rule for updating intentions
func (r *IntentionsReconciler) sourceIntention() *api.SourceIntention {
	return &api.SourceIntention{
		Name:        r.gatewayName.Name,
		Namespace:   r.gatewayName.Namespace,
		Action:      api.IntentionActionAllow,
		Description: fmt.Sprintf("Allow traffic from Consul API Gateway. Reconciled by controller at %s.", time.Now().Format(time.RFC3339)),
	}
}

// reconcileLoop runs until the struct context is cancelled, and is the main loop handling all intention reconciliation.
// Intentions can be reconciled if any one of three conditions are triggered:
//   1. Reconcile is called sending an error channel through the forceSyncChan. This will immediately attempt to sync
//      intentions and return the error through the received error channel, blocking until the error is handled or the
//      reconciler is stopped.
//   2. A discoChainWatcher sends a discoChainWatchResult which will compute and added or removed discovery chain
//      targets and synchronize intentions.
//   3. The intentionSyncInterval is met, triggering the a ticker to fire and synchronize intentions.
//
// The loop stops and returns if the struct context is cancelled.
func (r *IntentionsReconciler) reconcileLoop() {
	ticker := time.NewTicker(intentionSyncInterval)
	defer ticker.Stop()
	for {
		select {
		// return if the reconciler has been stopped
		case <-r.ctx.Done():
			return

		// handle calls to Reconcile which sends an error chan over the forceSyncChan
		case errCh := <-r.forceSyncChan:
			select {
			case errCh <- r.syncIntentions():
				// reset the ticker for full synchronization
				ticker.Reset(intentionSyncInterval)
				continue
			case <-r.ctx.Done():
				return
			}

		// remaining cases do not return or continue the loop which results in
		// r.syncIntentions being called

		// process changes to the compiled discovery chain
		case chain := <-r.discoChainChan:
			r.handleChainResult(chain)
			ticker.Reset(intentionSyncInterval)

		// periodically synchronize the intentions
		case <-ticker.C:
		}

		if err := r.syncIntentions(); err != nil {
			r.logger.Warn("one or more errors occurred during intention sync, some intentions may not have been updated", "error", err)
		}
	}
}

// updateChainWatchers is called when a change in the ingress gateway occurs to ensure
// each IngressService has a watcher running to handle updates in their discovery chain
func (r *IntentionsReconciler) updateChainWatchers(igw *api.IngressGatewayConfigEntry) {
	r.chainWatchersMutex.Lock()
	newIdx := ingressToServiceIndex(igw)
	added, removed := r.ingressServiceIndex.Diff(newIdx)
	for _, service := range removed {
		if watcher, ok := r.chainWatchers[service]; ok {
			watcher.Cancel()
			delete(r.chainWatchers, service)
		}
	}

	for _, service := range added {
		if _, ok := r.chainWatchers[service]; !ok {
			r.chainWatchers[service] = newDiscoChainWatcher(r.ctx, service, r.discoChainChan, r.consulDisco, r.logger)
		}
	}

	r.ingressServiceIndex = newIdx
	r.chainWatchersMutex.Unlock()
}

func ingressToServiceIndex(igw *api.IngressGatewayConfigEntry) *common.ServiceNameIndex {
	idx := common.NewServiceNameIndex()
	for _, lis := range igw.Listeners {
		for _, service := range lis.Services {
			idx.Add(api.CompoundServiceName{
				Name:      service.Name,
				Namespace: service.Namespace,
			})
		}
	}
	return idx
}

func (r *IntentionsReconciler) syncIntentions() error {
	mErr := &multierror.Error{}
	source := r.sourceIntention()
	addSourceCB := func(intention *api.ServiceIntentionsConfigEntry) bool {
		var found bool
		for _, src := range intention.Sources {
			if sourceIntentionMatches(src, source) {
				found = true
				break
			}
		}

		// add source to intention
		if !found {
			intention.Sources = append(intention.Sources, source)
			return true
		}
		return false
	}
	for _, target := range r.targetIndex.all() {
		if err := r.updateIntentionSources(target, addSourceCB); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("failed to update intention with added gateway source (%s/%s): %w", target.Namespace, target.Name, err))
		}
	}

	delSourceCB := func(intention *api.ServiceIntentionsConfigEntry) bool {
		for i, src := range intention.Sources {
			if sourceIntentionMatches(src, source) {
				intention.Sources = append(intention.Sources[:i], intention.Sources[i+1:]...)
				return true
			}
		}
		return false
	}
	for _, target := range r.targetTombstones.All() {
		if err := r.updateIntentionSources(target, delSourceCB); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("failed to update intention with removed gateway source (%s/%s): %w", target.Namespace, target.Name, err))
			continue
		}
		r.targetTombstones.Remove(target)
	}
	return mErr.ErrorOrNil()
}

// handleChain computes the added and removed targets from the last change
func (r *IntentionsReconciler) handleChainResult(result *discoChainWatchResult) {
	for _, target := range result.added {
		r.targetIndex.addRef(target, result.name)
		r.targetTombstones.Remove(target)
	}

	for _, target := range result.removed {
		r.targetIndex.delRef(target, result.name)
		if len(r.targetIndex.listRefs(target)) == 0 {
			r.targetTombstones.Add(target)
		}
	}
}

func (r *IntentionsReconciler) updateIntentionSources(name api.CompoundServiceName, updateFn func(intention *api.ServiceIntentionsConfigEntry) bool) error {
	if name.Namespace == api.IntentionDefaultNamespace {
		name.Namespace = ""
	}
	return backoff.Retry(func() error {
		intention, idx, err := r.getOrInitIntention(name)
		if err != nil {
			return err
		}

		// perform update on intention and check if any modifications have been reported
		if !updateFn(intention) {
			// if no intention changes stop here
			return nil
		}

		// if the intention now has no sources it can be deleted.
		// there is a race here where the intention could have been modified between the time it was read, potentially
		// removing the intention when it in fact still had Sources.
		// this issue tracks Consul's support for CAS for delete operations which would remove this race:
		// https://github.com/hashicorp/consul/issues/11372
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

// intentionTargetReferenceIndex tracks target service names and references to them.
// this is used to reference count service targets accross multiple discovery chains and ensure
// intentions are only removed when they have no references to them.
type intentionTargetReferenceIndex struct {
	refs map[api.CompoundServiceName]*common.ServiceNameIndex
}

// addRef adds a target to the index and tracks the reference from the source service
func (i *intentionTargetReferenceIndex) addRef(target, source api.CompoundServiceName) {
	if _, ok := i.refs[target]; !ok {
		i.refs[target] = common.NewServiceNameIndex()
	}
	i.refs[target].Add(source)
}

// delRef removes a target references from the index for the given source. If source was the final reference to
// the given target, the target is removed from the index.
func (i *intentionTargetReferenceIndex) delRef(target, source api.CompoundServiceName) {
	if _, ok := i.refs[target]; ok {
		i.refs[target].Remove(source)
		if len(i.refs[target].All()) == 0 {
			delete(i.refs, target)
		}
	}
}

// listRefs returns a slice of the referring sources for the given target
func (i *intentionTargetReferenceIndex) listRefs(target api.CompoundServiceName) []api.CompoundServiceName {
	var result []api.CompoundServiceName
	if _, ok := i.refs[target]; ok {
		result = i.refs[target].All()
	}
	return result
}

// all returns a slice of all target service in the index
func (i *intentionTargetReferenceIndex) all() []api.CompoundServiceName {
	var results []api.CompoundServiceName
	for name := range i.refs {
		results = append(results, name)
	}
	return results
}

func sourceIntentionMatches(a, b *api.SourceIntention) bool {
	canonicalizeDefault := func(s string) string {
		if s == "" {
			return api.IntentionDefaultNamespace
		}
		return s
	}
	return a.Name == b.Name && canonicalizeDefault(a.Namespace) == canonicalizeDefault(b.Namespace)
}
