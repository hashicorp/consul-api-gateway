package envoy

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/common"
)

//go:generate mockgen -source ./middleware.go -destination ./mocks/middleware.go -package mocks GatewayRegistry

type gatewayInfoKey struct{}

var gatewayInfoContextKey = &gatewayInfoKey{}

type wrappedStream struct {
	grpc.ServerStream
	wrappedContext context.Context
}

func (s *wrappedStream) Context() context.Context {
	return s.wrappedContext
}

func wrapStream(stream grpc.ServerStream, info *common.GatewayInfo) *wrappedStream {
	return &wrappedStream{
		ServerStream:   stream,
		wrappedContext: context.WithValue(stream.Context(), gatewayInfoContextKey, info),
	}
}

func GatewayFromContext(ctx context.Context) *common.GatewayInfo {
	value := ctx.Value(gatewayInfoContextKey)
	if value == nil {
		return nil
	}
	return value.(*common.GatewayInfo)
}

type GatewayRegistry interface {
	GatewayExists(info *common.GatewayInfo) bool
	CanFetchSecrets(info *common.GatewayInfo, secrets []string) bool
}

// SPIFFEStreamMiddleware verifies the spiffe entries for the certificate
// and sets the client identidy on the request context. If no
// spiffe information is detected, or if the service is unknown,
// the request is rejected.
func SPIFFEStreamMiddleware(logger hclog.Logger, spiffeCA *url.URL, registry GatewayRegistry) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info, ok := verifySPIFFE(ss.Context(), logger, spiffeCA, registry); ok {
			return handler(srv, wrapStream(ss, info))
		}
		return status.Errorf(codes.Unauthenticated, "unable to authenticate request")
	}
}

func verifySPIFFE(ctx context.Context, logger hclog.Logger, spiffeCA *url.URL, registry GatewayRegistry) (*common.GatewayInfo, bool) {
	if p, ok := peer.FromContext(ctx); ok {
		if mtls, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			for _, item := range mtls.State.PeerCertificates {
				for _, uri := range item.URIs {
					if uri.Scheme == "spiffe" {
						if uri.Host != spiffeCA.Host {
							logger.Warn("found mismatching spiffe hosts, skipping", "caHost", spiffeCA.Host, "clientHost", uri.Host)
							continue
						}
						info, err := parseURI(uri.Path)
						if err != nil {
							logger.Error("error parsing spiffe path, skipping", "error", err, "path", uri.Path)
							continue
						}
						if registry.GatewayExists(info) {
							return info, true
						}
					}
				}
			}
		}
	}
	return nil, false
}

func parseURI(path string) (*common.GatewayInfo, error) {
	path = strings.TrimPrefix(path, "/")
	tokens := strings.SplitN(path, "/", 6)
	if len(tokens) != 6 {
		return nil, errors.New("invalid spiffe path")
	}
	if tokens[0] != "ns" || tokens[2] != "dc" || tokens[4] != "svc" {
		return nil, errors.New("invalid spiffe path")
	}
	return &common.GatewayInfo{
		Namespace: tokens[1],
		Service:   tokens[5],
	}, nil
}
