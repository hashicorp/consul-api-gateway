package consul

import (
	"context"
	"time"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

// discoChainWatcher uses blocking queries to poll for changes in a services discovery chain
type discoChainWatcher struct {
	name api.CompoundServiceName

	ctx    context.Context
	cancel context.CancelFunc

	prevResults *common.ServiceNameIndex
	results     chan *discoChainWatchResult
	disco       consulDiscoveryChains

	logger hclog.Logger
}

func newDiscoChainWatcher(ctx context.Context, service api.CompoundServiceName, results chan *discoChainWatchResult, disco consulDiscoveryChains, logger hclog.Logger) *discoChainWatcher {
	child, cancel := context.WithCancel(ctx)
	w := &discoChainWatcher{
		ctx:         child,
		cancel:      cancel,
		prevResults: common.NewServiceNameIndex(),
		results:     results,
		disco:       disco,
		name:        service,
		logger:      logger,
	}
	go w.watchLoop()
	return w
}

func (w *discoChainWatcher) Cancel() {
	w.cancel()
}

type discoChainWatchResult struct {
	name    api.CompoundServiceName
	added   []api.CompoundServiceName
	removed []api.CompoundServiceName
}

func (w *discoChainWatcher) watchLoop() {
	var index uint64
	for {
		opts := &api.QueryOptions{WaitIndex: index, Namespace: w.name.Namespace}
		resp, meta, err := w.disco.Get(w.name.Name, nil, opts.WithContext(w.ctx))
		if err != nil {
			w.logger.Warn("blocking query for gateway discovery chain failed", "error", err)
			select {
			case <-w.ctx.Done():
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

		names := compiledDiscoChainToServiceNames(resp.Chain)
		added, removed := w.prevResults.Diff(names)
		result := &discoChainWatchResult{
			name:    w.name,
			added:   added,
			removed: removed,
		}

		select {
		case <-w.ctx.Done():
			return
		case w.results <- result:
			w.prevResults = names
		}
	}
}

func compiledDiscoChainToServiceNames(chain *api.CompiledDiscoveryChain) *common.ServiceNameIndex {
	names := common.NewServiceNameIndex()
	for _, target := range chain.Targets {
		if target.Service == chain.ServiceName && chain.Protocol != "tcp" {
			// chain targets include the chain service name
			// this service should only be included if the protocol is tcp
			continue
		}
		names.Add(api.CompoundServiceName{
			Name:      target.Service,
			Namespace: target.Namespace,
		})
	}
	return names
}
