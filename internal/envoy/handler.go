package envoy

import (
	"context"
	"fmt"
	"sync"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/metrics"
)

type RequestHandler struct {
	logger         hclog.Logger
	metrics        *metrics.SDSMetrics
	secretManager  *SecretManager
	nodeMap        sync.Map
	streamContexts sync.Map
}

func NewRequestHandler(logger hclog.Logger, metrics *metrics.SDSMetrics, secretManager *SecretManager) *server.CallbackFuncs {
	handler := &RequestHandler{
		metrics:       metrics,
		logger:        logger,
		secretManager: secretManager,
	}
	return &server.CallbackFuncs{
		DeltaStreamOpenFunc: func(ctx context.Context, streamID int64, typeURL string) error {
			logger.Trace("delta stream open")
			if typeURL != resource.SecretType {
				return fmt.Errorf("unsupported type: %s", typeURL)
			}
			return handler.OnDeltaStreamOpen(ctx, streamID)
		},
		DeltaStreamClosedFunc:  handler.OnDeltaStreamClosed,
		StreamDeltaRequestFunc: handler.OnStreamDeltaRequest,
	}
}

func (r *RequestHandler) OnDeltaStreamOpen(ctx context.Context, streamID int64) error {
	r.logger.Trace("beginning stream", "stream_id", streamID)
	r.streamContexts.Store(streamID, ctx)
	r.metrics.ActiveStreams.Inc()
	return nil
}

func (r *RequestHandler) OnDeltaStreamClosed(streamID int64) {
	r.logger.Trace("closing stream", "stream_id", streamID)
	if node, deleted := r.nodeMap.LoadAndDelete(streamID); deleted {
		r.logger.Trace("unwatching all secrets for node", "node", node.(string))
		r.secretManager.UnwatchAll(r.streamContext(streamID), node.(string))
	} else {
		r.logger.Warn("node not found for stream", "stream", streamID)
	}
	r.streamContexts.Delete(streamID)
	r.metrics.ActiveStreams.Dec()
}

func (r *RequestHandler) OnStreamDeltaRequest(streamID int64, req *discovery.DeltaDiscoveryRequest) error {
	ctx := r.streamContext(streamID)

	r.nodeMap.Store(streamID, req.Node.Id)
	if err := r.secretManager.Watch(ctx, req.ResourceNamesSubscribe, req.Node.Id); err != nil {
		return err
	}
	if err := r.secretManager.Unwatch(ctx, req.ResourceNamesUnsubscribe, req.Node.Id); err != nil {
		return nil
	}
	return nil
}

func (r *RequestHandler) streamContext(streamID int64) context.Context {
	if value, ok := r.streamContexts.Load(streamID); ok {
		return value.(context.Context)
	}
	return nil
}
