package middleware

// based on https://github.com/deepmap/oapi-codegen/blob/9dc8b8d293a991614ea12447bd6507bfadf38304/pkg/chi-middleware/oapi_validate.go
import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// ErrorHandler is called when there is an error in validation
type ErrorHandler func(w http.ResponseWriter, statusCode int, message string)

// Options to customize request validation, openapi3filter specified options will be passed through.
type Options struct {
	Options openapi3filter.Options
}

// OapiRequestValidator Creates middleware to validate request by swagger spec.
// This middleware is good for net/http either since go-chi is 100% compatible with net/http.
func OapiRequestValidator(swagger *openapi3.T, handler ErrorHandler) func(next http.Handler) http.Handler {
	return OapiRequestValidatorWithOptions(swagger, handler, nil)
}

// OapiRequestValidatorWithOptions Creates middleware to validate request by swagger spec.
// This middleware is good for net/http either since go-chi is 100% compatible with net/http.
func OapiRequestValidatorWithOptions(swagger *openapi3.T, handler ErrorHandler, options *Options) func(next http.Handler) http.Handler {
	router, err := gorillamux.NewRouter(swagger)
	if err != nil {
		panic(err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// validate request
			if statusCode, err := validateRequest(r, router, options); err != nil {
				handler(w, statusCode, err.Error())
				return
			}

			// serve
			next.ServeHTTP(w, r)
		})
	}

}

// This function is called from the middleware above and actually does the work
// of validating a request.
func validateRequest(r *http.Request, router routers.Router, options *Options) (int, error) {

	// Find route
	route, pathParams, err := router.FindRoute(r)
	if err != nil {
		return http.StatusBadRequest, err // We failed to find a matching route for the request.
	}

	// Validate request
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    r,
		PathParams: pathParams,
		Route:      route,
	}

	if options != nil {
		requestValidationInput.Options = &options.Options
	}

	if err := openapi3filter.ValidateRequest(context.Background(), requestValidationInput); err != nil {
		switch e := err.(type) {
		case *openapi3filter.RequestError:
			return formatSchemaError(e)
		case *openapi3filter.SecurityRequirementsError:
			return http.StatusUnauthorized, err
		default:
			// This should never happen today, but if our upstream code changes,
			// we don't want to crash the server, so handle the unexpected error.
			return http.StatusInternalServerError, fmt.Errorf("error validating route: %s", err.Error())
		}
	}

	return http.StatusOK, nil
}

func formatSchemaError(err *openapi3filter.RequestError) (int, error) {
	switch err := err.Unwrap().(type) {
	case *openapi3.SchemaError:
		message := strings.Join(err.JSONPointer(), ".")
		message += ": " + strings.ToLower(err.Reason)
		if err.SchemaField == "enum" {
			message += fmt.Sprintf("; allowed values: %s", enumToString(err.Schema.Enum))
		}
		if err.Value != nil && !isComplex(err.Value) {
			message += fmt.Sprintf("; current value: \"%v\"", err.Value)
		}
		return http.StatusBadRequest, errors.New(message)
	default:
		// we offer this fallback since we don't know exactly what the error is
		return http.StatusBadRequest, fmt.Errorf("error validating route: %s", err.Error())
	}
}

func enumToString(enum []interface{}) string {
	enumStrings := []string{}
	for _, e := range enum {
		enumStrings = append(enumStrings, fmt.Sprintf("%v", e))
	}
	return fmt.Sprintf("[%s]", strings.Join(enumStrings, ", "))
}

func isComplex(field interface{}) bool {
	kind := reflect.ValueOf(field).Kind()
	return kind == reflect.Map || kind == reflect.Array
}
