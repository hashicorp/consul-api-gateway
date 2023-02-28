// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package core

type TCPRoute struct {
	CommonRoute
	Service ResolvedService
}

func (r TCPRoute) GetType() ResolvedRouteType {
	return ResolvedTCPRouteType
}

type TCPRouteBuilder struct {
	meta      map[string]string
	name      string
	namespace string
	service   ResolvedService
}

func (b *TCPRouteBuilder) WithMeta(meta map[string]string) *TCPRouteBuilder {
	b.meta = meta
	return b
}

func (b *TCPRouteBuilder) WithName(name string) *TCPRouteBuilder {
	b.name = name
	return b
}

func (b *TCPRouteBuilder) WithNamespace(namespace string) *TCPRouteBuilder {
	b.namespace = namespace
	return b
}

func (b *TCPRouteBuilder) WithService(service ResolvedService) *TCPRouteBuilder {
	b.service = service
	return b
}

func (b *TCPRouteBuilder) Build() ResolvedRoute {
	return TCPRoute{
		CommonRoute: CommonRoute{
			Meta:      b.meta,
			Name:      b.name,
			Namespace: b.namespace,
		},
		Service: b.service,
	}
}

func NewTCPRouteBuilder() *TCPRouteBuilder {
	return &TCPRouteBuilder{}
}
