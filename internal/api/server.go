package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/hashicorp/consul-api-gateway/internal/api/apiinternal"
	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	consul "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type ServerConfig struct {
	Logger          hclog.Logger
	Consul          *consul.Client
	Address         string
	CertFile        string
	KeyFile         string
	ShutdownTimeout time.Duration

	// info for bootstrapping our deployments
	Bootstrap apiinternal.BootstrapConfiguration
}

type Server struct {
	server          *http.Server
	certFile        string
	keyFile         string
	shutdownTimeout time.Duration
}

func NewServer(config ServerConfig) *Server {
	router := chi.NewRouter()
	router.Mount("/api/v1", v1.NewServer("/api/v1", config.Consul, config.Logger))
	router.Mount("/api/internal", apiinternal.NewServer("/api/internal", config.Bootstrap, config.Consul, config.Logger))

	return &Server{
		server: &http.Server{
			Handler: router,
			Addr:    config.Address,
		},
		certFile:        config.CertFile,
		keyFile:         config.KeyFile,
		shutdownTimeout: config.ShutdownTimeout,
	}
}

// Run starts the API server
func (s *Server) Run(ctx context.Context) error {
	errs := make(chan error, 1)
	go func() {
		if s.certFile != "" && s.keyFile != "" {
			errs <- s.server.ListenAndServeTLS(s.certFile, s.keyFile)
		} else {
			errs <- s.server.ListenAndServe()
		}
	}()

	for {
		select {
		case err := <-errs:
			return err
		case <-ctx.Done():
			return s.Shutdown()
		}
	}
}

// Shutdown attempts to gracefully shutdown the server, it
// is called automatically when the context passed into the
// Run function is canceled.
func (s *Server) Shutdown() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()

		return s.server.Shutdown(ctx)
	}
	return nil
}
