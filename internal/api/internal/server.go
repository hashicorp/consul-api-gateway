package internal

import (
	"encoding/json"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/consul-api-gateway/internal/api/middleware"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
)

//go:generate oapi-codegen -config ../schemas/internal.config.yaml ../schemas/internal.yaml

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
	r.Use(middleware.JSONContentType, s.gatewayTokenMiddleware, middleware.OapiRequestValidator(spec, sendError))

	return HandlerWithOptions(s, ChiServerOptions{
		BaseURL:    url,
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			sendError(w, http.StatusBadRequest, err.Error())
		},
	}), nil
}

func sendError(w http.ResponseWriter, code int, message string) {
	send(w, code, Error{
		Code:    int32(code),
		Message: message,
	})
}

func send(w http.ResponseWriter, code int, object interface{}) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(object)
}

func sendEmpty(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
	w.Write([]byte("{}\n"))
}
