package v1

import (
	"context"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/consul-api-gateway/internal/api/middleware"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

//go:generate oapi-codegen -config ../schemas/v1.config.yaml ../schemas/v1.yaml
//go:generate oapi-codegen -old-config-style -package v1 -generate client -templates ../templates -o zz_generated_extensions.go ../schemas/v1.yaml

var _ ServerInterface = &Server{}

//go:generate mockgen -source ./server.go -destination ./zz_mocks_test.go -package v1 Validator
type Validator interface {
	ValidateGateway(ctx context.Context, gateway *Gateway) error
	ValidateHTTPRoute(ctx context.Context, route *HTTPRoute) error
	ValidateTCPRoute(ctx context.Context, route *TCPRoute) error
}

type Server struct {
	logger       hclog.Logger
	consulClient *api.Client
	validator    Validator
	name         string
	namespace    string
}

// TODO(andrew): most of this is boilerplate that should be generated

func NewServer(url string, validator Validator, name, namespace string, consulClient *api.Client, logger hclog.Logger) http.Handler {
	spec, _ := GetSwagger()
	spec.Servers = openapi3.Servers{&openapi3.Server{URL: url}}

	s := &Server{consulClient: consulClient, logger: logger, name: name, namespace: namespace, validator: validator}
	r := chi.NewRouter()
	r.Use(middleware.JSONContentType, s.consulTokenMiddleware, middleware.OapiRequestValidator(spec, sendError))

	return HandlerWithOptions(s, ChiServerOptions{
		BaseURL:    url,
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			sendError(w, http.StatusBadRequest, err.Error())
		},
	})
}
