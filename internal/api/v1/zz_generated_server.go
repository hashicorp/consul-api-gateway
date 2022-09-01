// Package v1 provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/andrewstucki/oapi-codegen version v1.10.2-0.20220902020913-b36ba463f350 DO NOT EDIT.
package v1

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/deepmap/oapi-codegen/pkg/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

// Defines values for ListenerProtocol.
const (
	Http ListenerProtocol = "http"
	Tcp  ListenerProtocol = "tcp"
)

// Certificate defines model for Certificate.
type Certificate struct {
	Vault *VaultCertificate `json:"vault,omitempty"`
}

// Error defines model for Error.
type Error struct {
	Code    int32  `json:"code"`
	Message string `json:"message"`
}

// Gateway defines model for Gateway.
type Gateway struct {
	Listeners []Listener             `json:"listeners"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
}

// GatewayPage defines model for GatewayPage.
type GatewayPage struct {
	Gateways []Gateway              `json:"gateways"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
}

// Listener defines model for Listener.
type Listener struct {
	Hostname string            `json:"hostname"`
	Name     *string           `json:"name,omitempty"`
	Port     float32           `json:"port"`
	Protocol ListenerProtocol  `json:"protocol"`
	Tls      *TLSConfiguration `json:"tls,omitempty"`
}

// ListenerProtocol defines model for Listener.Protocol.
type ListenerProtocol string

// TLSConfiguration defines model for TLSConfiguration.
type TLSConfiguration struct {
	Certificates []Certificate `json:"certificates,omitempty"`
	CipherSuites []string      `json:"cipherSuites,omitempty"`
	MaxVersion   *string       `json:"maxVersion,omitempty"`
	MinVersion   *string       `json:"minVersion,omitempty"`
}

// VaultCertificate defines model for VaultCertificate.
type VaultCertificate struct {
	ChainField      *string `json:"chainField,omitempty"`
	Path            *string `json:"path,omitempty"`
	PrivateKeyField *string `json:"privateKeyField,omitempty"`
}

// AddGatewayJSONRequestBody defines body for AddGateway for application/json ContentType.
type AddGatewayJSONRequestBody = Gateway

// ServerInterface represents all server handlers.
type ServerInterface interface {

	// (GET /gateways)
	ListGateways(w http.ResponseWriter, r *http.Request)

	// (POST /gateways)
	AddGateway(w http.ResponseWriter, r *http.Request)

	// (DELETE /gateways/{namespace}/{name})
	DeleteGateway(w http.ResponseWriter, r *http.Request, namespace string, name string)

	// (GET /gateways/{namespace}/{name})
	GetGateway(w http.ResponseWriter, r *http.Request, namespace string, name string)
}

// ServerInterfaceWrapper converts contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler            ServerInterface
	HandlerMiddlewares []MiddlewareFunc
	ErrorHandlerFunc   func(w http.ResponseWriter, r *http.Request, err error)
}

type MiddlewareFunc func(http.Handler) http.Handler

// ListGateways operation middleware
func (siw *ServerInterfaceWrapper) ListGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.ListGateways(w, r)
	})

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r.WithContext(ctx))
}

// AddGateway operation middleware
func (siw *ServerInterfaceWrapper) AddGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.AddGateway(w, r)
	})

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r.WithContext(ctx))
}

// DeleteGateway operation middleware
func (siw *ServerInterfaceWrapper) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var err error

	// ------------- Path parameter "namespace" -------------
	var namespace string

	err = runtime.BindStyledParameterWithLocation("simple", false, "namespace", runtime.ParamLocationPath, chi.URLParam(r, "namespace"), &namespace)
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "namespace", Err: err})
		return
	}

	// ------------- Path parameter "name" -------------
	var name string

	err = runtime.BindStyledParameterWithLocation("simple", false, "name", runtime.ParamLocationPath, chi.URLParam(r, "name"), &name)
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "name", Err: err})
		return
	}

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.DeleteGateway(w, r, namespace, name)
	})

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r.WithContext(ctx))
}

// GetGateway operation middleware
func (siw *ServerInterfaceWrapper) GetGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var err error

	// ------------- Path parameter "namespace" -------------
	var namespace string

	err = runtime.BindStyledParameterWithLocation("simple", false, "namespace", runtime.ParamLocationPath, chi.URLParam(r, "namespace"), &namespace)
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "namespace", Err: err})
		return
	}

	// ------------- Path parameter "name" -------------
	var name string

	err = runtime.BindStyledParameterWithLocation("simple", false, "name", runtime.ParamLocationPath, chi.URLParam(r, "name"), &name)
	if err != nil {
		siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "name", Err: err})
		return
	}

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siw.Handler.GetGateway(w, r, namespace, name)
	})

	for _, middleware := range siw.HandlerMiddlewares {
		handler = middleware(handler)
	}

	handler.ServeHTTP(w, r.WithContext(ctx))
}

type UnescapedCookieParamError struct {
	ParamName string
	Err       error
}

func (e *UnescapedCookieParamError) Error() string {
	return fmt.Sprintf("error unescaping cookie parameter '%s'", e.ParamName)
}

func (e *UnescapedCookieParamError) Unwrap() error {
	return e.Err
}

type UnmarshalingParamError struct {
	ParamName string
	Err       error
}

func (e *UnmarshalingParamError) Error() string {
	return fmt.Sprintf("Error unmarshaling parameter %s as JSON: %s", e.ParamName, e.Err.Error())
}

func (e *UnmarshalingParamError) Unwrap() error {
	return e.Err
}

type RequiredParamError struct {
	ParamName string
}

func (e *RequiredParamError) Error() string {
	return fmt.Sprintf("Query argument %s is required, but not found", e.ParamName)
}

type RequiredHeaderError struct {
	ParamName string
	Err       error
}

func (e *RequiredHeaderError) Error() string {
	return fmt.Sprintf("Header parameter %s is required, but not found", e.ParamName)
}

func (e *RequiredHeaderError) Unwrap() error {
	return e.Err
}

type InvalidParamFormatError struct {
	ParamName string
	Err       error
}

func (e *InvalidParamFormatError) Error() string {
	return fmt.Sprintf("Invalid format for parameter %s: %s", e.ParamName, e.Err.Error())
}

func (e *InvalidParamFormatError) Unwrap() error {
	return e.Err
}

type TooManyValuesForParamError struct {
	ParamName string
	Count     int
}

func (e *TooManyValuesForParamError) Error() string {
	return fmt.Sprintf("Expected one value for %s, got %d", e.ParamName, e.Count)
}

// Handler creates http.Handler with routing matching OpenAPI spec.
func Handler(si ServerInterface) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{})
}

type ChiServerOptions struct {
	BaseURL          string
	BaseRouter       chi.Router
	Middlewares      []MiddlewareFunc
	ErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, err error)
}

// HandlerFromMux creates http.Handler with routing matching OpenAPI spec based on the provided mux.
func HandlerFromMux(si ServerInterface, r chi.Router) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{
		BaseRouter: r,
	})
}

func HandlerFromMuxWithBaseURL(si ServerInterface, r chi.Router, baseURL string) http.Handler {
	return HandlerWithOptions(si, ChiServerOptions{
		BaseURL:    baseURL,
		BaseRouter: r,
	})
}

// HandlerWithOptions creates http.Handler with additional options
func HandlerWithOptions(si ServerInterface, options ChiServerOptions) http.Handler {
	r := options.BaseRouter

	if r == nil {
		r = chi.NewRouter()
	}
	if options.ErrorHandlerFunc == nil {
		options.ErrorHandlerFunc = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
	wrapper := ServerInterfaceWrapper{
		Handler:            si,
		HandlerMiddlewares: options.Middlewares,
		ErrorHandlerFunc:   options.ErrorHandlerFunc,
	}

	r.Group(func(r chi.Router) {
		r.Get(options.BaseURL+"/gateways", wrapper.ListGateways)
	})
	r.Group(func(r chi.Router) {
		r.Post(options.BaseURL+"/gateways", wrapper.AddGateway)
	})
	r.Group(func(r chi.Router) {
		r.Delete(options.BaseURL+"/gateways/{namespace}/{name}", wrapper.DeleteGateway)
	})
	r.Group(func(r chi.Router) {
		r.Get(options.BaseURL+"/gateways/{namespace}/{name}", wrapper.GetGateway)
	})

	return r
}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"H4sIAAAAAAAC/+RWTW/cNhD9KwTbo7xaOz3pVNdtDSPrwKjdoECQA02NJKYSyZAjb7aG/ntBUl+7VLIb",
	"NChc9KYVh/PevHkzq2fKVaOVBImWZs/U8goa5h+vwKAoBGcI7qc2Srs34A+fWFuje/jeQEEz+l065Un7",
	"JOlbFzRP03UJxZ0GmlH1+AE40i6hvxijTIzAVe5xC2UahjSjQuKrCzomEBKhBOMyNGAtK310I+QGZIkV",
	"zc7HUItGyNKjG/jYCgM5zd6N15KA9X6B2zVD2LJdzK4WFkGC8T8EQmOPibHpb3jCQt6EOxNJZgzbhWqQ",
	"uWQRGcma4zWGMKsZ/2o9fP75/WRW5hfUueu131eoDIenCzRo3Z0syQH/EXGJ6yh/RLRSFgdpF8VcPNDK",
	"4OxAts1j6K02ChVXtTsE2TaOWYWonXO5nnGbcmF9VJyHzf2VkoUoW8NQKBkVP1bRU5sRWZIjyhfP3zS3",
	"p/dwb9jjPnKhKzD3rTjMGUty6AD26S0Y2zONwhshP3+8tHSi1RTXXzEhfxVQ5yfMnGbu5HiYEU8M4TXs",
	"TkscM+8SaoG3RuDu3mkeuF5q8Rp2l20gISTNaAUsB0MHB9M/zq6UtG199qD+BDmtUeav0s5lFrJQYfVK",
	"ZNzbGxomapq5V+4y0+KsH7MfK2YrwZXRK66aCSjAkMu7G9JPNHkA5gJa4zK5WcjSdP92l9AcLDdCBzMu",
	"ZbllkpXQgET32i8nDtL61vXYt+ovUdeM3LWPteBkEwLIxWq9B2+zNN1ut6smhK+UKVOQZ7/fp7d3m/Ri",
	"tU69BQXWywXRhD4NbqPnq/Vq7eKVBsm0oBl95V8FV/j+pPNlWIJXdr/e3wBbIy1hdU14P5iQk+Heivr8",
	"YVZvcpr5fXY9ZHWrwGolbbDDxXo9tBGkB2Na187nQsn0gw1TEmb2xK3sd7w3yT7vniAZ8EMni+Hr4JtQ",
	"CB8IC+CthE8aOEJOYIrRyi4IfGXAbTLCiITtoGss62WeT012+xUs/qTy3bfWc6mcweioCMtzOt/vaFro",
	"oi6f/xusXlyHu2Sap/R5/FrpwnMXWl9D2On76cJ7ZwIrZFnD4APyyCzkREmCFZA3rAFiW1cN5JFDfvY5",
	"JpNoZlgD6D8F3x0CvhnYEVWMYKhIAcgrmoRV7f88xv05//zaN0AyUzP6o1hCPgDtVfks6lcBvo/c+EOs",
	"9wAekPMXsBy+uHxd8w8cURRORIGWtFJ8bIE4oeK1cQ34X3TEEdB/aIj1/3M9dd3fAQAA//9LI94bVQ8A",
	"AA==",
}

// GetSwagger returns the content of the embedded swagger specification file
// or error if failed to decode
func decodeSpec() ([]byte, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %s", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}

	return buf.Bytes(), nil
}

var rawSpec = decodeSpecCached()

// a naive cached of a decoded swagger spec
func decodeSpecCached() func() ([]byte, error) {
	data, err := decodeSpec()
	return func() ([]byte, error) {
		return data, err
	}
}

// Constructs a synthetic filesystem for resolving external references when loading openapi specifications.
func PathToRawSpec(pathToFile string) map[string]func() ([]byte, error) {
	var res = make(map[string]func() ([]byte, error))
	if len(pathToFile) > 0 {
		res[pathToFile] = rawSpec
	}

	return res
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file. The external references of Swagger specification are resolved.
// The logic of resolving external references is tightly connected to "import-mapping" feature.
// Externally referenced files must be embedded in the corresponding golang packages.
// Urls can be supported but this task was out of the scope.
func GetSwagger() (swagger *openapi3.T, err error) {
	var resolvePath = PathToRawSpec("")

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		var pathToFile = url.String()
		pathToFile = path.Clean(pathToFile)
		getSpec, ok := resolvePath[pathToFile]
		if !ok {
			err1 := fmt.Errorf("path not found: %s", pathToFile)
			return nil, err1
		}
		return getSpec()
	}
	var specData []byte
	specData, err = rawSpec()
	if err != nil {
		return
	}
	swagger, err = loader.LoadFromData(specData)
	if err != nil {
		return
	}
	return
}
