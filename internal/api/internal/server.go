package internal

import (
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/consul-api-gateway/internal/api/middleware"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

//go:generate oapi-codegen -config ../schemas/internal.config.yaml ../schemas/internal.yaml
//go:generate oapi-codegen -old-config-style -package internal -generate client -templates ../templates -o zz_generated_extensions.go ../schemas/internal.yaml

var _ ServerInterface = &Server{}

type Server struct {
	logger       hclog.Logger
	consulClient *api.Client
}

// TODO(andrew): most of this is boilerplate that should be generated

func NewServer(url string, consulClient *api.Client, logger hclog.Logger) http.Handler {
	spec, _ := GetSwagger()
	spec.Servers = openapi3.Servers{&openapi3.Server{URL: url}}

	s := &Server{consulClient: consulClient, logger: logger}
	r := chi.NewRouter()
	r.Use(middleware.JSONContentType, s.gatewayTokenMiddleware, middleware.OapiRequestValidator(spec, sendError))

	return HandlerWithOptions(s, ChiServerOptions{
		BaseURL:    url,
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			sendError(w, http.StatusBadRequest, err.Error())
		},
	})
}
