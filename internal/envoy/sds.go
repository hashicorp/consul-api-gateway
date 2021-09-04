package envoy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"time"

	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"google.golang.org/grpc/grpclog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/consul"
)

const (
	defaultGRPCPort        = ":9090"
	defaultShutdownTimeout = 10 * time.Second
)

type SDSStream = envoy_secret_v3.SecretDiscoveryService_StreamSecretsServer
type SDSDelta = envoy_secret_v3.SecretDiscoveryService_DeltaSecretsServer

type SDSServer struct {
	logger  hclog.Logger
	manager *consul.CertManager
	server  *grpc.Server
}

func NewSDSServer(logger hclog.Logger, manager *consul.CertManager) *SDSServer {
	return &SDSServer{
		logger:  logger,
		manager: manager,
	}
}

func (s *SDSServer) DeltaSecrets(stream SDSDelta) error {
	s.logger.Info("Got DeltaSecrets request")
	return nil
}

func (s *SDSServer) StreamSecrets(stream SDSStream) error {
	s.logger.Info("Got StreamSecrets request")
	return nil
}

func (s *SDSServer) FetchSecrets(context context.Context, request *envoy_discovery_v3.DiscoveryRequest) (*envoy_discovery_v3.DiscoveryResponse, error) {
	s.logger.Info("Got FetchSecrets request")
	return nil, nil
}

// GRPC returns a server instance that can handle xDS requests.
func (s *SDSServer) Run(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(os.Stdout, os.Stdout, os.Stdout, 2))

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
			RootCAs: certPool,
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
		})),
	}
	s.server = grpc.NewServer(opts...)
	envoy_secret_v3.RegisterSecretDiscoveryServiceServer(s.server, s)

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
