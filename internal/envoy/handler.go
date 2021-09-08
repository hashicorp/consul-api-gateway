package envoy

import (
	"context"
	"fmt"
	"sync/atomic"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"

	"github.com/hashicorp/go-hclog"
)

type RequestHandler struct {
	logger        hclog.Logger
	secretManager *SecretManager
	activeStreams int64
}

func NewRequestHandler(logger hclog.Logger, secretManager *SecretManager) server.Callbacks {
	handler := &RequestHandler{
		logger:        logger,
		secretManager: secretManager,
	}
	return server.CallbackFuncs{
		DeltaStreamOpenFunc: func(ctx context.Context, streamID int64, typeURL string) error {
			if typeURL != resource.SecretType {
				return fmt.Errorf("unsupported type: %s", typeURL)
			}
			return handler.OnDeltaStreamOpen(ctx, streamID)
		},
		DeltaStreamClosedFunc:   handler.OnDeltaStreamClosed,
		StreamDeltaRequestFunc:  handler.OnStreamDeltaRequest,
		StreamDeltaResponseFunc: handler.OnStreamDeltaResponse,
	}
}

func (r *RequestHandler) OnDeltaStreamOpen(ctx context.Context, streamID int64) error {
	// this should register the stream with the poller
	atomic.AddInt64(&r.activeStreams, 1)
	return nil
}

func (r *RequestHandler) OnDeltaStreamClosed(streamID int64) {
	r.secretManager.UnwatchAll(streamID)
	atomic.AddInt64(&r.activeStreams, -1)
}

func (r *RequestHandler) OnStreamDeltaRequest(streamID int64, req *discovery.DeltaDiscoveryRequest) error {
	// this should un-register the secret polling, which, in turn, should remove items
	// from the cache
	return nil
}

func (r *RequestHandler) OnStreamDeltaResponse(streamID int64, req *discovery.DeltaDiscoveryRequest, resp *discovery.DeltaDiscoveryResponse) {
	// add diagnostics here
}
