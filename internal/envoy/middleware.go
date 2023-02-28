// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package envoy

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
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

func wrapStream(stream grpc.ServerStream, info core.GatewayID) *wrappedStream {
	return &wrappedStream{
		ServerStream:   stream,
		wrappedContext: context.WithValue(stream.Context(), gatewayInfoContextKey, info),
	}
}

// GatewayFromContext retrieves info about a gateway from the context or nil if there is none
func GatewayFromContext(ctx context.Context) core.GatewayID {
	value := ctx.Value(gatewayInfoContextKey)
	if value == nil {
		return core.GatewayID{}
	}
	return value.(core.GatewayID)
}

// SPIFFEStreamMiddleware verifies the spiffe entries for the certificate
// and sets the client identidy on the request context. If no
// spiffe information is detected, or if the service is unknown,
// the request is rejected.
func SPIFFEStreamMiddleware(logger hclog.Logger, store store.ReadStore) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info, ok := verifySPIFFE(ss.Context(), logger, store); ok {
			return handler(srv, wrapStream(ss, info))
		}
		return status.Errorf(codes.Unauthenticated, "unable to authenticate request")
	}
}

func verifySPIFFE(ctx context.Context, logger hclog.Logger, store store.ReadStore) (core.GatewayID, bool) {
	if p, ok := peer.FromContext(ctx); ok {
		if mtls, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			// grab the peer certificate info
			for _, item := range mtls.State.PeerCertificates {
				// check each untyped SAN for spiffee information
				for _, uri := range item.URIs {
					// we skip validating the host value since not all of our potential roots (i.e. via Vault)
					// have spiffe urls
					if uri.Scheme == "spiffe" {
						// make sure we have a leaf certificate that has been issued by consul
						// with namespace, datacenter, and service information -- the namespace
						// and service are used to inform us what gateway is trying to connect
						info, err := parseURI(uri.Path)
						if err != nil {
							logger.Error("error parsing spiffe path, skipping", "error", err, "path", uri.Path)
							continue
						}
						// if we're tracking the gateway then we're good
						gateway, err := store.GetGateway(ctx, info)
						if err != nil {
							logger.Error("error checking for gateway, skipping", "error", err)
							continue
						}
						if gateway != nil {
							return info, true
						}
						logger.Warn("gateway not found", "namespace", info.ConsulNamespace, "service", info.Service)
					}
				}
			}
		}
	}
	return core.GatewayID{}, false
}

func parseURI(path string) (core.GatewayID, error) {
	path = strings.TrimPrefix(path, "/")
	tokens := strings.SplitN(path, "/", 6)
	if len(tokens) != 6 {
		return core.GatewayID{}, errors.New("invalid spiffe path")
	}
	if tokens[0] != "ns" || tokens[2] != "dc" || tokens[4] != "svc" {
		return core.GatewayID{}, errors.New("invalid spiffe path")
	}
	namespace := tokens[1]
	if namespace == "default" {
		namespace = ""
	}
	return core.GatewayID{
		ConsulNamespace: namespace,
		Service:         tokens[5],
	}, nil
}
