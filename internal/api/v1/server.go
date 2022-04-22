package v1

import (
	"encoding/json"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/consul-api-gateway/internal/api/middleware"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

//go:generate oapi-codegen -generate "spec,types,chi-server" -package v1 -o zz_generated_server.go ../schemas/v1.json

var _ ServerInterface = &Server{}

type Server struct {
	logger       hclog.Logger
	consulClient *api.Client
}

func NewServer(url string, consulClient *api.Client, logger hclog.Logger) (http.Handler, error) {
	spec, err := GetSwagger()
	if err != nil {
		return nil, err
	}
	spec.Servers = openapi3.Servers{&openapi3.Server{URL: url}}

	s := &Server{consulClient: consulClient, logger: logger}
	r := chi.NewRouter()
	r.Use(middleware.JSONContentType, s.consulTokenMiddleware, middleware.OapiRequestValidator(spec, s.sendError))

	return HandlerWithOptions(s, ChiServerOptions{
		BaseURL:    url,
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			s.sendError(w, http.StatusBadRequest, err.Error())
		},
	}), nil
}

func (s *Server) sendError(w http.ResponseWriter, statusCode int, msg string) {
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(&Error{Message: msg}); err != nil {
		s.logger.Error("sending error message", "error", err)
	}
}

func (s *Server) FindGateways(w http.ResponseWriter, r *http.Request) {
	s.sendError(w, http.StatusNotImplemented, "Not implemented")
}
