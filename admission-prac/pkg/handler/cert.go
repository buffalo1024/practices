package handler

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	webhookNamespace  = "test"
	webhookService    = "test-mutate-webhook"
	Organization      = "noorganization"
	DefaultEffecttime = 10
	dnsNames          = []string{webhookService, webhookService + "." + webhookNamespace, webhookService + "." + webhookNamespace + "." + "svc"}
	commonName        = webhookService + "." + webhookNamespace + "." + "svc"
	certsDir          = "/etc/webhook/certs"
	certKey           = "tls.key"
	certFile          = "tls.crt"
)

type certManager struct {
	Organizations []string      `json:"organizations"`
	EffectiveTime time.Duration `json:"effectiveTime"`
	DNSNames      []string      `json:"DNSNames"`
	CommonName    string        `json:"commonName"`
}

func NewCertManager(
	Orz []string,
	effectiveTime time.Duration,
	dnsNames []string,
	commonName string) *certManager {
	return &certManager{
		Organizations: Orz,
		EffectiveTime: effectiveTime,
		DNSNames:      dnsNames,
		CommonName:    commonName,
	}
}

func HandleCerts() (serverCertPEM *bytes.Buffer, serverPrivateKeyPEM *bytes.Buffer, err error) {
	serverCertPEM, serverPrivateKeyPEM, err = NewCertManager(
		[]string{Organization},
		time.Until(time.Date(time.Now().Year()+DefaultEffecttime, time.Now().Month(), time.Now().Day(), time.Now().Hour(), time.Now().Minute(), 0, 0, time.Now().Location())),
		dnsNames,
		commonName,
	).GenerateSelfSignedCerts()
	if err != nil {
		logrus.WithError(err).Error("failed to generate certs")
		// return err
		return
	}

	err = os.MkdirAll(certsDir, 0666)
	if err != nil {
		logrus.WithField("certDir", certsDir).WithError(err).Error("failed to create cert dir")
		// return err
		return
	}

	err = WriteFile(filepath.Join(certsDir, certFile), serverCertPEM)
	if err != nil {
		logrus.WithField("tls.cert", serverCertPEM.String()).WithError(err).Error("failed to write tls.cert")
		// return err
		return
	}

	err = WriteFile(filepath.Join(certsDir, certKey), serverPrivateKeyPEM)
	if err != nil {
		logrus.WithField("tls.key", serverPrivateKeyPEM.String()).WithError(err).Error("failed to write tls.key")
		// return err
		return
	}

	// if err = CreateAdmissionConfig(serverCertPEM); err != nil {
	// 	log.WithField("tls.cert", serverCertPEM.String()).WithError(err).Error("failed to create admission config")
	// 	return err
	// }

	// return nil

	return
}

func (m *certManager) GenerateSelfSignedCerts() (serverCertPEM *bytes.Buffer, serverPrivateKeyPEM *bytes.Buffer, err error) {
	// CA config
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2021),
		Subject: pkix.Name{
			Organization: m.Organizations,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(m.EffectiveTime),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	var caPrivateKey *rsa.PrivateKey
	caPrivateKey, err = rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		logrus.WithError(err).Error("failed to generate key")
		return
	}

	// self signed CA certificate
	var caBytes []byte
	caBytes, err = x509.CreateCertificate(cryptorand.Reader, ca, ca, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		logrus.WithError(err).Error("failed to create certs")
		return
	}

	// PEM encode CA cert
	var caPEM = new(bytes.Buffer)
	_ = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	// server cert config
	cert := &x509.Certificate{
		DNSNames:     m.DNSNames,
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			CommonName:   m.CommonName,
			Organization: m.Organizations,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// server private key
	var serverPrivateKey *rsa.PrivateKey
	serverPrivateKey, err = rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		logrus.WithError(err).Error("failed to generate server private key")
		return
	}

	// sign the server cert
	var serverCertBytes []byte
	serverCertBytes, err = x509.CreateCertificate(cryptorand.Reader, cert, ca, &serverPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		logrus.WithError(err).Error("failed to generate server public cert")
		return
	}

	// PEM encode the server cert and key
	serverCertPEM = new(bytes.Buffer)
	_ = pem.Encode(serverCertPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertBytes,
	})
	serverPrivateKeyPEM = new(bytes.Buffer)
	_ = pem.Encode(serverPrivateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivateKey),
	})

	return
}

func WriteFile(filepath string, sCert *bytes.Buffer) error {
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(sCert.Bytes())
	if err != nil {
		return err
	}

	return nil
}
