package testing

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"time"

	"github.com/google/uuid"
)

var (
	DefaultTestCA                *CertificateInfo
	DefaultTestServerCertificate *CertificateInfo
	DefaultTestClientCertificate *CertificateInfo
)

func init() {
	rootCA, err := GenerateSignedCertificate(GenerateCertificateOptions{
		IsCA: true,
	})
	if err != nil {
		panic(err)
	}
	serverCert, err := GenerateSignedCertificate(GenerateCertificateOptions{
		CA:          rootCA,
		ServiceName: "server",
	})
	if err != nil {
		panic(err)
	}
	clientCert, err := GenerateSignedCertificate(GenerateCertificateOptions{
		CA:          rootCA,
		ServiceName: "client",
	})
	if err != nil {
		panic(err)
	}
	DefaultTestCA = rootCA
	DefaultTestServerCertificate = serverCert
	DefaultTestClientCertificate = clientCert
}

// CertificateInfo wraps all of the information needed to describe a generated
// certificate
type CertificateInfo struct {
	Cert            *x509.Certificate
	CertBytes       []byte
	PrivateKey      *rsa.PrivateKey
	PrivateKeyBytes []byte
	X509            tls.Certificate
	spiffe          *url.URL
}

// DefaultCertificates returns an initially generated CA, server, and client certificate
func DefaultCertificates() (*CertificateInfo, *CertificateInfo, *CertificateInfo) {
	return DefaultTestCA, DefaultTestServerCertificate, DefaultTestClientCertificate
}

func getSVIDRootURI() *url.URL {
	var svid url.URL
	svid.Scheme = "spiffe"
	svid.Host = uuid.New().String() + ".consul"
	return &svid
}

func getSVIDServiceURI(root *url.URL, hostOverride, pathOverride, service string) *url.URL {
	var svid url.URL
	svid.Scheme = "spiffe"
	svid.Host = root.Host
	if hostOverride != "" {
		svid.Host = hostOverride
	}
	svid.Path = "/ns/default/dc/testing/svc/" + service
	if pathOverride != "" {
		svid.Path = pathOverride
	}
	return &svid
}

// GenerateCertificateOptions describe how he want to generate a certificate
type GenerateCertificateOptions struct {
	CA                 *CertificateInfo
	IsCA               bool
	ServiceName        string
	SPIFFEHostOverride string
	SPIFFEPathOverride string
	ExtraSANs          []string
	ExtraIPs           []net.IP
	Expiration         time.Time
	Bits               int
}

// GenerateSignedCertificate generates a certificate with the given options
func GenerateSignedCertificate(options GenerateCertificateOptions) (*CertificateInfo, error) {
	bits := options.Bits
	if bits == 0 {
		bits = 1024
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}
	usage := x509.KeyUsageDigitalSignature
	if options.IsCA {
		usage = x509.KeyUsageCertSign
	}

	spiffe := getSVIDRootURI()
	if options.CA != nil {
		spiffe = getSVIDServiceURI(options.CA.spiffe, options.SPIFFEHostOverride, options.SPIFFEPathOverride, options.ServiceName)
	}
	expiration := options.Expiration
	if expiration.IsZero() {
		expiration = time.Now().AddDate(10, 0, 0)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		URIs:         []*url.URL{spiffe},
		DNSNames:     options.ExtraSANs,
		Subject: pkix.Name{
			Organization:  []string{"Testing, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Fake Street"},
			PostalCode:    []string{"11111"},
		},
		IsCA:                  options.IsCA,
		IPAddresses:           append(options.ExtraIPs, net.IPv4(127, 0, 0, 1), net.IPv6loopback),
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              expiration,
		SubjectKeyId:          []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              usage,
		BasicConstraintsValid: true,
	}
	caCert := cert
	if options.CA != nil {
		caCert = options.CA.Cert
	}
	caPrivateKey := privateKey
	if options.CA != nil {
		caPrivateKey = options.CA.PrivateKey
	}
	data, err := x509.CreateCertificate(rand.Reader, cert, caCert, &privateKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}

	var certificatePEM bytes.Buffer
	var privateKeyPEM bytes.Buffer
	if err := pem.Encode(&certificatePEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: data,
	}); err != nil {
		return nil, err
	}
	if err := pem.Encode(&privateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		return nil, err
	}

	certBytes := certificatePEM.Bytes()
	privateKeyBytes := privateKeyPEM.Bytes()
	x509Cert, err := tls.X509KeyPair(certBytes, privateKeyBytes)
	if err != nil {
		return nil, err
	}

	return &CertificateInfo{
		Cert:            cert,
		CertBytes:       certBytes,
		PrivateKey:      privateKey,
		PrivateKeyBytes: privateKeyBytes,
		X509:            x509Cert,
		spiffe:          spiffe,
	}, nil
}
