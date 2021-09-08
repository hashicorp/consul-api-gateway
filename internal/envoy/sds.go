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
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	server "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/hashicorp/polar/internal/consul"
	polarGRPC "github.com/hashicorp/polar/internal/grpc"
)

const (
	defaultGRPCPort        = ":9090"
	defaultShutdownTimeout = 10 * time.Second
)

type SDSServer struct {
	logger  hclog.Logger
	manager *consul.CertManager
	server  *grpc.Server

	stopCtx context.Context
}

func NewSDSServer(logger hclog.Logger, manager *consul.CertManager) *SDSServer {
	return &SDSServer{
		logger:  logger,
		manager: manager,
	}
}

// GRPC returns a server instance that can handle xDS requests.
func (s *SDSServer) Run(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	s.stopCtx = childCtx
	defer cancel()

	grpclog.SetLoggerV2(polarGRPC.NewHCLogLogger(s.logger))

	rootCA, err := s.manager.RootCA()
	if err != nil {
		return err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(rootCA) {
		return fmt.Errorf("failed to add server CA's certificate")
	}

	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(2048),
		grpc.Creds(credentials.NewTLS(&tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				cert, err := s.manager.Certificate()
				if err != nil {
					return nil, err
				}
				privateKey, err := s.manager.PrivateKey()
				if err != nil {
					return nil, err
				}
				certificate, err := tls.X509KeyPair(cert, privateKey)
				if err != nil {
					return nil, err
				}
				return &certificate, nil
			},
			ClientCAs:  certPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
		})),
	}
	s.server = grpc.NewServer(opts...)

	resourceCache := cache.NewLinearCache(resource.SecretType)
	secretManager := NewSecretManager(&stubSecretClient{}, "", resourceCache, s.logger.Named("secret-manager"))
	handler := NewRequestHandler(s.logger.Named("handler"), secretManager)
	sdsServer := server.NewServer(childCtx, resourceCache, handler)
	secretservice.RegisterSecretDiscoveryServiceServer(s.server, sdsServer)

	listener, err := net.Listen("tcp", defaultGRPCPort)
	if err != nil {
		return err
	}

	go func() {
		<-childCtx.Done()
		s.Shutdown()
	}()
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
