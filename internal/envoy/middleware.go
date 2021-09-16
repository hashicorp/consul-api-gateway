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

//go:generate mockgen -source ./middleware.go -destination ./mocks/middleware.go -package mocks GatewaySecretRegistry

type contextKey string

const (
	gatewayInfoContextKey = contextKey("gatewayInfo")
)

type wrappedStream struct {
	grpc.ServerStream
	wrappedContext context.Context
}

func (s *wrappedStream) Context() context.Context {
	return s.wrappedContext
}

func wrapStream(stream grpc.ServerStream, info common.GatewayInfo) *wrappedStream {
	return &wrappedStream{
		ServerStream:   stream,
		wrappedContext: context.WithValue(stream.Context(), gatewayInfoContextKey, info),
	}
}

// GatewayFromContext retrieves info about a gateway from the context or nil if there is none
func GatewayFromContext(ctx context.Context) common.GatewayInfo {
	value := ctx.Value(gatewayInfoContextKey)
	if value == nil {
		return common.GatewayInfo{}
	}
	return value.(common.GatewayInfo)
}

// GatewaySecretRegistry is used as the authority for determining what gateways the SDS server
// should actually respond to because they're managed by polar
type GatewaySecretRegistry interface {
	// GatewayExists is used to determine whether or not we know a particular gateway instance
	GatewayExists(info common.GatewayInfo) bool
	// CanFetchSecrets is used to determine whether a gateway should be able to fetch a set
	// of secrets it has requested
	CanFetchSecrets(info common.GatewayInfo, secrets []string) bool
}

// SPIFFEStreamMiddleware verifies the spiffe entries for the certificate
// and sets the client identidy on the request context. If no
// spiffe information is detected, or if the service is unknown,
// the request is rejected.
func SPIFFEStreamMiddleware(logger hclog.Logger, spiffeCA *url.URL, registry GatewaySecretRegistry) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info, ok := verifySPIFFE(ss.Context(), logger, spiffeCA, registry); ok {
			return handler(srv, wrapStream(ss, info))
		}
		return status.Errorf(codes.Unauthenticated, "unable to authenticate request")
	}
}

func verifySPIFFE(ctx context.Context, logger hclog.Logger, spiffeCA *url.URL, registry GatewaySecretRegistry) (common.GatewayInfo, bool) {
	if p, ok := peer.FromContext(ctx); ok {
		if mtls, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			// grab the peer certificate info
			for _, item := range mtls.State.PeerCertificates {
				// check each untyped SAN for spiffee information
				for _, uri := range item.URIs {
					if uri.Scheme == "spiffe" {
						// we've found a spiffee SAN, check that it aligns with the CA info
						if uri.Host != spiffeCA.Host {
							logger.Warn("found mismatching spiffe hosts, skipping", "caHost", spiffeCA.Host, "clientHost", uri.Host)
							continue
						}
						// make sure we have a leaf certificate that has been issued by consul
						// with namespace, datacenter, and service information -- the namespace
						// and service are used to inform us what gateway is trying to connect
						info, err := parseURI(uri.Path)
						if err != nil {
							logger.Error("error parsing spiffe path, skipping", "error", err, "path", uri.Path)
							continue
						}
						// if we're tracking the gateway then we're good
						if registry.GatewayExists(info) {
							return info, true
						}
					}
				}
			}
		}
	}
	return common.GatewayInfo{}, false
}

func parseURI(path string) (common.GatewayInfo, error) {
	path = strings.TrimPrefix(path, "/")
	tokens := strings.SplitN(path, "/", 6)
	if len(tokens) != 6 {
		return common.GatewayInfo{}, errors.New("invalid spiffe path")
	}
	if tokens[0] != "ns" || tokens[2] != "dc" || tokens[4] != "svc" {
		return common.GatewayInfo{}, errors.New("invalid spiffe path")
	}
	return common.GatewayInfo{
		Namespace: tokens[1],
		Service:   tokens[5],
	}, nil
}
