package common

var defaultTLSCipherSuites = []string{
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
}

func DefaultTLSCipherSuites() []string {
	return defaultTLSCipherSuites
}

// NOTE: the following cipher suites are currently supported by Envoy but insecure and
// pending removal
var extraTLSCipherSuites = []string{
	// https://github.com/envoyproxy/envoy/issues/5399
	"TLS_RSA_WITH_AES_128_GCM_SHA256",
	"TLS_RSA_WITH_AES_128_CBC_SHA",
	"TLS_RSA_WITH_AES_256_GCM_SHA384",
	"TLS_RSA_WITH_AES_256_CBC_SHA",

	// https://github.com/envoyproxy/envoy/issues/5400
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
}

var supportedTLSCipherSuites = (func() map[string]struct{} {
	cipherSuites := make(map[string]struct{})

	for _, c := range append(defaultTLSCipherSuites, extraTLSCipherSuites...) {
		cipherSuites[c] = struct{}{}
	}

	return cipherSuites
})()

func SupportedTLSCipherSuite(cipherSuite string) bool {
	_, ok := supportedTLSCipherSuites[cipherSuite]
	return ok
}

var SupportedTLSVersions = map[string]struct{}{
	"TLS_AUTO": {},
	"TLSv1_0":  {},
	"TLSv1_1":  {},
	"TLSv1_2":  {},
	"TLSv1_3":  {},
}

var TLSVersionsWithConfigurableCipherSuites = map[string]struct{}{
	// Remove these two if Envoy ever sets TLS 1.3 as default minimum
	"":         {},
	"TLS_AUTO": {},

	"TLSv1_0": {},
	"TLSv1_1": {},
	"TLSv1_2": {},
}
