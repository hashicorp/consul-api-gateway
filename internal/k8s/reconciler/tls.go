package reconciler

const (
	annotationKeyPrefix          = "api-gateway.consul.hashicorp.com/"
	tlsMinVersionAnnotationKey   = annotationKeyPrefix + "tls_min_version"
	tlsMaxVersionAnnotationKey   = annotationKeyPrefix + "tls_max_version"
	tlsCipherSuitesAnnotationKey = annotationKeyPrefix + "tls_cipher_suites"
)

var supportedTlsVersions = map[string]struct{}{
	"TLS_AUTO": {},
	"TLSv1_0":  {},
	"TLSv1_1":  {},
	"TLSv1_2":  {},
	"TLSv1_3":  {},
}

var tlsVersionsWithConfigurableCipherSuites = map[string]struct{}{
	// Remove these two if Envoy ever sets TLS 1.3 as default minimum
	"":         {},
	"TLS_AUTO": {},

	"TLSv1_0": {},
	"TLSv1_1": {},
	"TLSv1_2": {},
}
