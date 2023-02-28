// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validator

const (
	annotationKeyPrefix          = "api-gateway.consul.hashicorp.com/"
	tlsMinVersionAnnotationKey   = annotationKeyPrefix + "tls_min_version"
	tlsMaxVersionAnnotationKey   = annotationKeyPrefix + "tls_max_version"
	tlsCipherSuitesAnnotationKey = annotationKeyPrefix + "tls_cipher_suites"
)
