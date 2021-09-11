package envoy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/log"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/health"
	healthservice "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/hashicorp/go-hclog"

	polarGRPC "github.com/hashicorp/polar/internal/grpc"
	"github.com/hashicorp/polar/internal/metrics"
)

//go:generate mockgen -source ./sds.go -destination ./mocks/sds.go -package mocks CertificateFetcher

const (
	defaultGRPCBindAddress = ":9090"
	defaultShutdownTimeout = 10 * time.Second
)

type CertificateFetcher interface {
	RootCA() []byte
	TLSCertificate() *tls.Certificate
}

type SDSServer struct {
	logger      hclog.Logger
	fetcher     CertificateFetcher
	metrics     *metrics.SDSMetrics
	server      *grpc.Server
	client      SecretClient
	bindAddress string
	protocol    string
}

func NewSDSServer(logger hclog.Logger, metrics *metrics.SDSMetrics, fetcher CertificateFetcher, client SecretClient) *SDSServer {
	return &SDSServer{
		logger:      logger,
		fetcher:     fetcher,
		metrics:     metrics,
		client:      client,
		bindAddress: defaultGRPCBindAddress,
		protocol:    "tcp",
	}
}

// GRPC returns a server instance that can handle xDS requests.
func (s *SDSServer) Run(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	grpclog.SetLoggerV2(polarGRPC.NewHCLogLogger(s.logger))

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(s.fetcher.RootCA()) {
		return fmt.Errorf("failed to add server CA's certificate")
	}

	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(2048),
		grpc.Creds(credentials.NewTLS(&tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				return s.fetcher.TLSCertificate(), nil
			},
			ClientCAs:  certPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
		})),
	}
	s.server = grpc.NewServer(opts...)

	resourceCache := cache.NewLinearCache(resource.SecretType, cache.WithLogger(wrapEnvoyLogger(s.logger.Named("cache"))))
	secretManager := NewSecretManager(s.client, resourceCache, s.logger.Named("secret-manager"))
	handler := NewRequestHandler(s.logger.Named("handler"), s.metrics, secretManager)
	sdsServer := server.NewServer(childCtx, resourceCache, handler)
	secretservice.RegisterSecretDiscoveryServiceServer(s.server, sdsServer)
	healthservice.RegisterHealthServer(s.server, health.NewServer())
	listener, err := net.Listen(s.protocol, s.bindAddress)
	if err != nil {
		return err
	}

	go func() {
		secretManager.Manage(childCtx)
	}()
	go func() {
		<-childCtx.Done()
		s.Shutdown()
	}()
	go func() {
		for {
			select {
			case <-childCtx.Done():
				return
			case <-time.After(10 * time.Second):
				resources := len(resourceCache.GetResources())
				s.metrics.CachedResources.Set(float64(resources))
			}
		}
	}()

	s.logger.Trace("running SDS server")
	return s.server.Serve(listener)
}

func (s *SDSServer) Shutdown() {
	if s.server != nil {
		stopped := make(chan struct{})
		go func() {
			s.server.GracefulStop()
			close(stopped)
		}()

		timer := time.NewTimer(defaultShutdownTimeout)
		select {
		case <-timer.C:
			s.server.Stop()
		case <-stopped:
			timer.Stop()
		}
	}
}

func logFunc(log func(msg string, args ...interface{})) func(msg string, args ...interface{}) {
	return func(msg string, args ...interface{}) {
		log(fmt.Sprintf(msg, args...))
	}
}

func wrapEnvoyLogger(logger hclog.Logger) log.Logger {
	return log.LoggerFuncs{
		DebugFunc: logFunc(logger.Debug),
		InfoFunc:  logFunc(logger.Info),
		WarnFunc:  logFunc(logger.Warn),
		ErrorFunc: logFunc(logger.Error),
	}
}
