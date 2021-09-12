package envoy

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

//go:generate mockgen -source ./middleware.go -destination ./mocks/middleware.go -package mocks GatewayRegistry

type GatewayRegistry interface {
	GatewayExists(namespace, datacenter, service string) bool
}

type stubGatewayRegistry struct{}

func (m *stubGatewayRegistry) GatewayExists(namespace, datacenter, service string) bool {
	return true
}

// SPIFFEStreamMiddleware verifies the spiffe entries for the certificate
// and sets the client identidy on the request context. If no
// spiffe information is detected, or if the service is unknown,
// the request is rejected.
func SPIFFEStreamMiddleware(logger hclog.Logger, spiffeCA *url.URL, registry GatewayRegistry) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if verifySPIFFE(ss.Context(), spiffeCA, registry) {
			return handler(srv, ss)
		}
		return status.Errorf(codes.Unauthenticated, "unable to authenticate request")
	}
}

func SPIFFEUnaryMiddleware(logger hclog.Logger, spiffeCA *url.URL, registry GatewayRegistry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if verifySPIFFE(ctx, spiffeCA, registry) {
			return handler(ctx, req)
		}
		return nil, status.Errorf(codes.Unauthenticated, "unable to authenticate request")
	}
}

func verifySPIFFE(ctx context.Context, spiffeCA *url.URL, registry GatewayRegistry) bool {
	if p, ok := peer.FromContext(ctx); ok {
		if mtls, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			for _, item := range mtls.State.PeerCertificates {
				for _, uri := range item.URIs {
					if uri.Scheme == "spiffe" && uri.Host == spiffeCA.Host {
						ns, dc, svc, err := parseURI(uri.Path)
						if err != nil {
							continue
						}
						if registry.GatewayExists(ns, dc, svc) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func parseURI(path string) (string, string, string, error) {
	path = strings.TrimPrefix(path, "/")
	tokens := strings.SplitN(path, "/", 6)
	if len(tokens) != 6 {
		return "", "", "", errors.New("invalid spiffe path")
	}
	if tokens[0] != "ns" || tokens[2] != "dc" || tokens[4] != "svc" {
		return "", "", "", errors.New("invalid spiffe path")
	}
	return tokens[1], tokens[3], tokens[5], nil
}
