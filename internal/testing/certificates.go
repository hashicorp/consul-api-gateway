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
	"time"
)

var (
	DefaultTestCA                *CertificateInfo
	DefaultTestServerCertificate *CertificateInfo
	DefaultTestClientCertificate *CertificateInfo
)

func init() {
	rootCA, err := GenerateSignedCertificate(nil, true)
	if err != nil {
		panic(err)
	}
	serverCert, err := GenerateSignedCertificate(rootCA, false)
	if err != nil {
		panic(err)
	}
	clientCert, err := GenerateSignedCertificate(rootCA, false)
	if err != nil {
		panic(err)
	}
	DefaultTestCA = rootCA
	DefaultTestServerCertificate = serverCert
	DefaultTestClientCertificate = clientCert
}

type CertificateInfo struct {
	Cert            *x509.Certificate
	CertBytes       []byte
	PrivateKey      *rsa.PrivateKey
	PrivateKeyBytes []byte
	X509            tls.Certificate
}

func DefaultCertificates() (*CertificateInfo, *CertificateInfo, *CertificateInfo) {
	return DefaultTestCA, DefaultTestServerCertificate, DefaultTestClientCertificate
}

func GenerateSignedCertificate(ca *CertificateInfo, isCA bool) (*CertificateInfo, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Testing, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Fake Street"},
			PostalCode:    []string{"11111"},
		},
		IsCA:                  isCA,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		SubjectKeyId:          []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caCert := cert
	if ca != nil {
		caCert = ca.Cert
	}
	caPrivateKey := privateKey
	if ca != nil {
		caPrivateKey = ca.PrivateKey
	}
	data, err := x509.CreateCertificate(rand.Reader, cert, caCert, &privateKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}

	var certificatePEM bytes.Buffer
	var privateKeyPEM bytes.Buffer
	pem.Encode(&certificatePEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: data,
	})
	pem.Encode(&privateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

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
	}, nil
}
