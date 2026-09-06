package pki

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDevelopmentCAAndCSR(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(dir, "ca-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if err = Init(dir); err == nil {
		t.Fatal("must not overwrite existing CA")
	}
	current, _ := os.ReadFile(filepath.Join(dir, "ca-key.pem"))
	if string(original) != string(current) {
		t.Fatal("CA replaced")
	}
	ca, err := Load(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	key, _, _ := Key()
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		t.Fatal(err)
	}
	data := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
	csr, err := ParseCSR(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseCSR(data + "garbage"); err == nil {
		t.Fatal("CSR trailing content accepted")
	}
	der[len(der)-1] ^= 1
	if _, err = ParseCSR(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))); err == nil {
		t.Fatal("invalid signature accepted")
	}
	ca.Certificate.NotAfter = time.Now().Add(time.Hour)
	if _, _, _, err = ca.Issue("test", csr, time.Now()); err == nil {
		t.Fatal("certificate outliving issuer accepted")
	}
}
