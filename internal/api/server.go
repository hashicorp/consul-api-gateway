package api

import (
	"net/http"

	"github.com/go-chi/chi"
	"github.com/hashicorp/consul-api-gateway/internal/api/internal"
	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	consul "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

type ServerConfig struct {
	Logger  hclog.Logger
	Consul  *consul.Client
	Address string
}

// TODO(andrew) should this be generated too?

func NewServer(config ServerConfig) *http.Server {
	router := chi.NewRouter()
	router.Mount("/api/v1", v1.NewServer("/api/v1", config.Consul, config.Logger))
	router.Mount("/api/internal", internal.NewServer("/api/internal", config.Consul, config.Logger))

	return &http.Server{
		Handler: router,
		Addr:    config.Address,
	}
}
