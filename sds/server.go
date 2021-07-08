package sds

import (
	"context"
	"net"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

const (
	sdsTypeURI = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret"
)

type Server struct {
	addr   string
	logger hclog.Logger

	cache *cache.LinearCache
}

func NewServer(addr string, logger hclog.Logger) *Server {
	return &Server{
		addr:   addr,
		logger: logger,
		cache:  cache.NewLinearCache(sdsTypeURI),
	}
}

func (s *Server) Serve(ctx context.Context) error {
	// Serve xDS
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	var callbacks *xds.CallbackFuncs
	if s.logger != nil {
		callbacks = makeLoggerCallbacks(s.logger.Named("sds-server"))
	}

	xdsServer := xds.NewServer(ctx, s.cache, callbacks)
	grpcServer := grpc.NewServer()

	secretservice.RegisterSecretDiscoveryServiceServer(grpcServer, xdsServer)

	// TODO is this a reasonable way to stop the server? Not sure graceful stop
	// matters?
	go func() {
		<-ctx.Done()
		grpcServer.Stop()
	}()

	return grpcServer.Serve(l)
}

func (s *Server) UpdateTLSSecret(name string, certChain, key []byte) error {
	var res tls.Secret
	res.Name = name
	res.Type = &tls.Secret_TlsCertificate{
		TlsCertificate: &tls.TlsCertificate{
			CertificateChain: &core.DataSource{
				Specifier: &core.DataSource_InlineBytes{
					InlineBytes: certChain,
				},
			},
			PrivateKey: &core.DataSource{
				Specifier: &core.DataSource_InlineBytes{
					InlineBytes: key,
				},
			},
		},
	}

	return s.cache.UpdateResource(name, types.Resource(&res))
}

func makeLoggerCallbacks(log hclog.Logger) *xds.CallbackFuncs {
	return &xds.CallbackFuncs{
		StreamOpenFunc: func(_ context.Context, id int64, addr string) error {
			log.Trace("gRPC stream opened", "id", id, "addr", addr)
			return nil
		},
		StreamClosedFunc: func(id int64) {
			log.Trace("gRPC stream closed", "id", id)
		},
		StreamRequestFunc: func(id int64, req *discovery.DiscoveryRequest) error {
			log.Trace("gRPC stream request", "id", id,
				"node.id", req.Node.Id,
				"req.typeURL", req.TypeUrl,
				"req.version", req.VersionInfo,
			)
			return nil
		},
		StreamResponseFunc: func(id int64, req *discovery.DiscoveryRequest, resp *discovery.DiscoveryResponse) {
			log.Trace("gRPC stream request", "id", id,
				"resp.typeURL", resp.TypeUrl,
				"resp.version", resp.VersionInfo,
			)
		},
	}
}
