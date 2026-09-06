package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Authority struct {
	Certificate *x509.Certificate
	Key         *ecdsa.PrivateKey
}

func serial() (*big.Int, error) { return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128)) }

func Key() (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	return key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), err
}

func ParseKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid private key")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("expected EC private key")
	}
	return key, nil
}

func Load(certPath, keyPath string) (*Authority, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("invalid CA")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	key, err := ParseKey(keyPEM)
	if err != nil {
		return nil, err
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok || !pub.Equal(&key.PublicKey) || !cert.IsCA {
		return nil, errors.New("invalid CA key pair")
	}
	return &Authority{cert, key}, nil
}

// Init creates a development-only CA and server certificate. It never replaces files.
func Init(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	key, keyPEM, err := Key()
	if err != nil {
		return err
	}
	n, err := serial()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	ca := &x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: "Prisme development CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(1, 0, 0), IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	serverKey, serverPEM, err := Key()
	if err != nil {
		return err
	}
	n, err = serial()
	if err != nil {
		return err
	}
	server := &x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: "Prisme development API"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(0, 1, 0), DNSNames: []string{"localhost", "api"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err = x509.CreateCertificate(rand.Reader, server, ca, &serverKey.PublicKey, key)
	if err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{"ca.pem", caPEM}, {"ca-key.pem", keyPEM}, {"server.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, {"server-key.pem", serverPEM}} {
		if err = WriteNew(filepath.Join(dir, f.name), f.data); err != nil {
			return err
		}
	}
	return nil
}

func WriteNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func ParseCSR(data string) (*x509.CertificateRequest, error) {
	block, rest := pem.Decode([]byte(data))
	if block == nil || len(rest) != 0 || block.Type != "CERTIFICATE REQUEST" {
		return nil, errors.New("invalid CSR")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, err
	}
	if err = csr.CheckSignature(); err != nil {
		return nil, err
	}
	pub, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, errors.New("P-256 key required")
	}
	return csr, nil
}

// Issue ignores all requested subject/SAN/extensions; identity is assigned by the API.
func (a *Authority) Issue(id string, csr *x509.CertificateRequest, now time.Time) (string, string, time.Time, error) {
	n, err := serial()
	if err != nil {
		return "", "", time.Time{}, err
	}
	expires := now.Add(24 * time.Hour)
	if expires.After(a.Certificate.NotAfter) {
		return "", "", time.Time{}, errors.New("CA expires too soon")
	}
	cert := &x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: id}, NotBefore: now.Add(-time.Minute), NotAfter: expires, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, a.Certificate, csr.PublicKey, a.Key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), n.String(), expires, err
}
