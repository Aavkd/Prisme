package pki

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Server names seen by Caddy and the internal probes: lowercase DNS labels only.
var serverName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

func validServer(names []string, days int) error {
	if len(names) == 0 || len(names) > 8 || days < 1 || days > 825 {
		return errors.New("invalid server certificate request")
	}
	for _, n := range names {
		if len(n) > 253 || !serverName.MatchString(n) {
			return errors.New("invalid server name")
		}
	}
	return nil
}

// InitProduction creates a dedicated control-plane CA and a server certificate
// for the given names. Like Init, it never replaces existing files.
func InitProduction(dir, name string, names []string, caYears, serverDays int) error {
	if name == "" || len(name) > 64 || caYears < 1 || caYears > 10 {
		return errors.New("invalid CA request")
	}
	if err := validServer(names, serverDays); err != nil {
		return err
	}
	for _, f := range []string{"ca.pem", "ca-key.pem", "server.pem", "server-key.pem"} {
		if _, err := os.Lstat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			return errors.New("destination already holds a PKI; refusing to replace it")
		}
	}
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
	tmpl := &x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: name}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(caYears, 0, 0), IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}
	certPEM, serverKeyPEM, err := issueServer(&Authority{ca, key}, names, serverDays, now)
	if err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{"ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, {"ca-key.pem", keyPEM}, {"server.pem", certPEM}, {"server-key.pem", serverKeyPEM}} {
		if err = WriteNew(filepath.Join(dir, f.name), f.data); err != nil {
			return err
		}
	}
	return nil
}

// RenewServer issues a new server key and certificate from the CA in dir and
// replaces server.pem/server-key.pem. The CA and agent identities are kept; the
// API reads the pair at startup, so it must be restarted afterwards.
func RenewServer(dir string, names []string, serverDays int) (time.Time, error) {
	if err := validServer(names, serverDays); err != nil {
		return time.Time{}, err
	}
	ca, err := Load(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca-key.pem"))
	if err != nil {
		return time.Time{}, errors.New("CA unavailable")
	}
	now := time.Now().UTC()
	certPEM, keyPEM, err := issueServer(ca, names, serverDays, now)
	if err != nil {
		return time.Time{}, err
	}
	// Both new files are complete on disk before either one replaces the old pair.
	for _, f := range []struct {
		name string
		data []byte
	}{{"server-key.pem", keyPEM}, {"server.pem", certPEM}} {
		tmp := filepath.Join(dir, f.name+".new")
		_ = os.Remove(tmp)
		if err = WriteNew(tmp, f.data); err != nil {
			return time.Time{}, err
		}
	}
	for _, f := range []string{"server-key.pem", "server.pem"} {
		if err = os.Rename(filepath.Join(dir, f+".new"), filepath.Join(dir, f)); err != nil {
			return time.Time{}, err
		}
	}
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	return cert.NotAfter, nil
}

func issueServer(ca *Authority, names []string, days int, now time.Time) ([]byte, []byte, error) {
	key, keyPEM, err := Key()
	if err != nil {
		return nil, nil, err
	}
	n, err := serial()
	if err != nil {
		return nil, nil, err
	}
	expires := now.AddDate(0, 0, days)
	if expires.After(ca.Certificate.NotAfter) {
		return nil, nil, errors.New("CA expires before the server certificate")
	}
	tmpl := &x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: names[0]}, NotBefore: now.Add(-time.Minute), NotAfter: expires, DNSNames: names, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Certificate, &key.PublicKey, ca.Key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), keyPEM, nil
}
