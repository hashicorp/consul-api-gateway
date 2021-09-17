package envoy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/metrics"
)

// RequestHandler implements the handlers for an SDS Delta server
type RequestHandler struct {
	logger         hclog.Logger
	secretManager  SecretManager
	registry       GatewaySecretRegistry
	nodeMap        sync.Map
	streamContexts sync.Map
	activeStreams  int64
}

// NewRequestHandler initializes a RequestHandler instance and wraps it in a github.com/envoyproxy/go-control-plane/pkg/server/v3,(*CallbackFuncs)
// so that it can be used by the stock go-control-plane server implementation
func NewRequestHandler(logger hclog.Logger, registry GatewaySecretRegistry, secretManager SecretManager) *server.CallbackFuncs {
	handler := &RequestHandler{
		registry:      registry,
		logger:        logger,
		secretManager: secretManager,
	}
	return &server.CallbackFuncs{
		DeltaStreamOpenFunc: func(ctx context.Context, streamID int64, typeURL string) error {
			logger.Trace("delta stream open")
			// make sure we're only responding to requests for secrets (we're an SDS server)
			if typeURL != resource.SecretType {
				return fmt.Errorf("unsupported type: %s", typeURL)
			}
			return handler.OnDeltaStreamOpen(ctx, streamID)
		},
		DeltaStreamClosedFunc:  handler.OnDeltaStreamClosed,
		StreamDeltaRequestFunc: handler.OnStreamDeltaRequest,
	}
}

// OnDeltaStreamOpen is invoked when an envoy instance first connects to the server
func (r *RequestHandler) OnDeltaStreamOpen(ctx context.Context, streamID int64) error {
	r.logger.Trace("beginning stream", "stream_id", streamID)
	// store the context, because we're never given it again
	r.streamContexts.Store(streamID, ctx)

	activeStreams := atomic.AddInt64(&r.activeStreams, 1)
	metrics.Registry.SetGauge(metrics.SDSActiveStreams, float32(activeStreams))
	return nil
}

// OnDeltaStreamClosed is invoked when an envoy instance disconnects from the server
func (r *RequestHandler) OnDeltaStreamClosed(streamID int64) {
	r.logger.Trace("closing stream", "stream_id", streamID)

	if node, deleted := r.nodeMap.LoadAndDelete(streamID); deleted {
		r.logger.Trace("unwatching all secrets for node", "node", node.(string))
		if err := r.secretManager.UnwatchAll(r.streamContext(streamID), node.(string)); err != nil {
			r.logger.Error("error unwatching secrets", "node", node.(string), "error", err)
		}
	} else {
		r.logger.Warn("node not found for stream", "stream", streamID)
	}
	if _, deleted := r.streamContexts.LoadAndDelete(streamID); deleted {
		activeStreams := atomic.AddInt64(&r.activeStreams, -1)
		metrics.Registry.SetGauge(metrics.SDSActiveStreams, float32(activeStreams))
	}
}

// OnStreamDeltaRequest is invoked when a request for resources comes in from the envoy instance
func (r *RequestHandler) OnStreamDeltaRequest(streamID int64, req *discovery.DeltaDiscoveryRequest) error {
	ctx := r.streamContext(streamID)

	// check to make sure we're actually authorized to do this
	if !r.registry.CanFetchSecrets(GatewayFromContext(ctx), req.ResourceNamesSubscribe) {
		return status.Errorf(codes.PermissionDenied, "the current gateway does not have permission to fetch the requested secrets")
	}

	// store the node information that we use to communicate with the manager
	// this is the only time we get the node id
	r.nodeMap.Store(streamID, req.Node.Id)
	if err := r.secretManager.Watch(ctx, req.ResourceNamesSubscribe, req.Node.Id); err != nil {
		return err
	}
	if err := r.secretManager.Unwatch(ctx, req.ResourceNamesUnsubscribe, req.Node.Id); err != nil {
		return err
	}
	return nil
}

func (r *RequestHandler) streamContext(streamID int64) context.Context {
	if value, ok := r.streamContexts.Load(streamID); ok {
		return value.(context.Context)
	}
	return context.Background()
}
