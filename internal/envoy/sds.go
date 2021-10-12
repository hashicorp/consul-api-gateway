package envoy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/log"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/hashicorp/go-hclog"

	grpcint "github.com/hashicorp/consul-api-gateway/internal/grpc"
	"github.com/hashicorp/consul-api-gateway/internal/metrics"
)

//go:generate mockgen -source ./sds.go -destination ./mocks/sds.go -package mocks CertificateFetcher

const (
	defaultGRPCBindAddress = ":9090"
	defaultShutdownTimeout = 10 * time.Second

	cachedMetricsTimeout = 10 * time.Second
)

var logOnce sync.Once

// CertificateFetcher is used to fetch the CA and server certificate
// that the server should use for TLS
type CertificateFetcher interface {
	SPIFFE() *url.URL
	RootPool() *x509.CertPool
	TLSCertificate() *tls.Certificate
}

// SDSServer wraps a gRPC-based SDS Delta server
type SDSServer struct {
	logger          hclog.Logger
	fetcher         CertificateFetcher
	server          *grpc.Server
	client          SecretClient
	bindAddress     string
	protocol        string
	gatewayRegistry GatewaySecretRegistry
}

// NEWSDSServer initializes an SDSServer instance
func NewSDSServer(logger hclog.Logger, fetcher CertificateFetcher, client SecretClient, registry GatewaySecretRegistry) *SDSServer {
	return &SDSServer{
		logger:          logger,
		fetcher:         fetcher,
		client:          client,
		bindAddress:     defaultGRPCBindAddress,
		protocol:        "tcp",
		gatewayRegistry: registry,
	}
}

// Run starts the SDS server
func (s *SDSServer) Run(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	logOnce.Do(func() {
		grpclog.SetLoggerV2(grpcint.NewHCLogLogger(s.logger))
	})

	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(2048),
		grpc.Creds(credentials.NewTLS(&tls.Config{
			GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
				certificate := s.fetcher.TLSCertificate()
				if certificate == nil {
					return nil, errors.New("unable to get server certificate")
				}
				pool := s.fetcher.RootPool()
				if pool == nil {
					return nil, errors.New("unable to get CA pool")
				}
				return &tls.Config{
					Certificates: []tls.Certificate{*certificate},
					ClientCAs:    pool,
					ClientAuth:   tls.RequireAndVerifyClientCert,
				}, nil
			},
			ClientAuth: tls.RequireAndVerifyClientCert,
		})),
		grpc.StreamInterceptor(SPIFFEStreamMiddleware(s.logger, s.fetcher, s.gatewayRegistry)),
	}
	s.server = grpc.NewServer(opts...)

	resourceCache := cache.NewLinearCache(resource.SecretType, cache.WithLogger(wrapEnvoyLogger(s.logger.Named("cache"))))
	secretManager := NewSecretManager(s.client, resourceCache, s.logger.Named("secret-manager"))
	handler := NewRequestHandler(s.logger.Named("handler"), s.gatewayRegistry, secretManager)
	sdsServer := server.NewServer(childCtx, resourceCache, handler)
	secretservice.RegisterSecretDiscoveryServiceServer(s.server, sdsServer)
	listener, err := net.Listen(s.protocol, s.bindAddress)
	if err != nil {
		return err
	}

	go secretManager.Manage(childCtx)
	errs := make(chan error, 1)
	go func() {
		errs <- s.server.Serve(listener)
	}()

	s.logger.Trace("running SDS server")
	for {
		select {
		case err := <-errs:
			return err
		case <-childCtx.Done():
			s.Shutdown()
			return nil
		case <-time.After(cachedMetricsTimeout):
			resources := len(resourceCache.GetResources())
			metrics.Registry.SetGauge(metrics.SDSCachedResources, float32(resources))
		}
	}
}

// Shutdown attempts to gracefully shutdown the server, it
// is called automatically when the context passed into the
// Run function is canceled.
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
