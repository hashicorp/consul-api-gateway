// Package v1 provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/andrewstucki/oapi-codegen version v1.10.2-0.20220902020913-b36ba463f350 DO NOT EDIT.
package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Start of User Generated Client

type UnexpectedResponse struct {
	code int
	body string
}

func NewUnexpectedResponse(code int, body []byte) *UnexpectedResponse {
	return &UnexpectedResponse{
		code: code,
		body: string(body),
	}
}

func (e *UnexpectedResponse) Error() string {
	return fmt.Sprintf("server response could not be parsed - code: %d, message: %s", e.code, e.body)
}

func (e *Error) Error() string {
	return fmt.Sprintf("server error - code: %d, message: %s", e.Code, e.Message)
}

func IsNotFound(e error) bool {
	var err *Error
	if errors.As(e, &err) {
		return err.Code == http.StatusNotFound
	}
	return false
}

func (c *APIClient) ListGateways(ctx context.Context) (*GatewayPage, error) {
	resp, err := c.client.ListGatewaysWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.JSONDefault != nil {
		return nil, resp.JSONDefault
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, NewUnexpectedResponse(resp.StatusCode(), resp.Body)
}

func (c *APIClient) DeleteGateway(ctx context.Context, namespace string, name string) error {
	resp, err := c.client.DeleteGatewayWithResponse(ctx, namespace, name)
	if err != nil {
		return err
	}
	if resp.JSONDefault != nil && resp.JSONDefault.Code != 0 {
		return resp.JSONDefault
	}
	if resp.StatusCode() != http.StatusAccepted {
		return NewUnexpectedResponse(resp.StatusCode(), resp.Body)
	}
	return nil
}

func (c *APIClient) GetGateway(ctx context.Context, namespace string, name string) (*Gateway, error) {
	resp, err := c.client.GetGatewayWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.JSONDefault != nil {
		return nil, resp.JSONDefault
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, NewUnexpectedResponse(resp.StatusCode(), resp.Body)
}

func (c *APIClient) ListHTTPRoutes(ctx context.Context) (*HTTPRoutePage, error) {
	resp, err := c.client.ListHTTPRoutesWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.JSONDefault != nil {
		return nil, resp.JSONDefault
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, NewUnexpectedResponse(resp.StatusCode(), resp.Body)
}

func (c *APIClient) DeleteHTTPRoute(ctx context.Context, namespace string, name string) error {
	resp, err := c.client.DeleteHTTPRouteWithResponse(ctx, namespace, name)
	if err != nil {
		return err
	}
	if resp.JSONDefault != nil && resp.JSONDefault.Code != 0 {
		return resp.JSONDefault
	}
	if resp.StatusCode() != http.StatusAccepted {
		return NewUnexpectedResponse(resp.StatusCode(), resp.Body)
	}
	return nil
}

func (c *APIClient) GetHTTPRoute(ctx context.Context, namespace string, name string) (*HTTPRoute, error) {
	resp, err := c.client.GetHTTPRouteWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.JSONDefault != nil {
		return nil, resp.JSONDefault
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, NewUnexpectedResponse(resp.StatusCode(), resp.Body)
}

func (c *APIClient) ListTCPRoutes(ctx context.Context) (*TCPRoutePage, error) {
	resp, err := c.client.ListTCPRoutesWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.JSONDefault != nil {
		return nil, resp.JSONDefault
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, NewUnexpectedResponse(resp.StatusCode(), resp.Body)
}

func (c *APIClient) DeleteTCPRoute(ctx context.Context, namespace string, name string) error {
	resp, err := c.client.DeleteTCPRouteWithResponse(ctx, namespace, name)
	if err != nil {
		return err
	}
	if resp.JSONDefault != nil && resp.JSONDefault.Code != 0 {
		return resp.JSONDefault
	}
	if resp.StatusCode() != http.StatusAccepted {
		return NewUnexpectedResponse(resp.StatusCode(), resp.Body)
	}
	return nil
}

func (c *APIClient) GetTCPRoute(ctx context.Context, namespace string, name string) (*TCPRoute, error) {
	resp, err := c.client.GetTCPRouteWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.JSONDefault != nil {
		return nil, resp.JSONDefault
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, NewUnexpectedResponse(resp.StatusCode(), resp.Body)
}

// End of User Generated Client

// Start generated server helpers

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

// End generated server helpers