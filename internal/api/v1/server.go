package v1

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/consul-api-gateway/internal/api/middleware"
	"github.com/hashicorp/go-hclog"
)

//go:generate oapi-codegen -generate "types,chi-server" -package v1 -o zz_generated_server.go ../schemas/v1.json

var _ ServerInterface = &Server{}

type Server struct {
	logger hclog.Logger
}

func NewServer(url string, logger hclog.Logger) http.Handler {
	s := &Server{logger: logger}
	r := chi.NewRouter()
	r.Use(middleware.JSONContentType)
	return HandlerWithOptions(s, ChiServerOptions{
		BaseURL:    url,
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			s.sendError(w, http.StatusBadRequest, err.Error())
		},
	})
}

func (s *Server) sendError(w http.ResponseWriter, statusCode int, msg string) {
	jsonEncoder := json.NewEncoder(w)
	w.WriteHeader(statusCode)
	err := jsonEncoder.Encode(&Error{Message: msg})
	if err != nil {
		s.logger.Error("sending error message", "error", err)
	}
}

func (s *Server) FindGateways(w http.ResponseWriter, r *http.Request) {
	s.sendError(w, http.StatusNotImplemented, "Not implemented")
}
