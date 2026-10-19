package pki

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProductionPKIServesNamesAndRenewsWithoutTouchingCA(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pki")
	names := []string{"prisme-api", "localhost"}
	if err := InitProduction(dir, "Prisme control plane CA", names, 5, 397); err != nil {
		t.Fatal(err)
	}
	if err := InitProduction(dir, "Prisme control plane CA", names, 5, 397); err == nil {
		t.Fatal("existing PKI replaced")
	}
	caKey, _ := os.ReadFile(filepath.Join(dir, "ca-key.pem"))
	verify := func() *x509.Certificate {
		t.Helper()
		pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.pem"), filepath.Join(dir, "server-key.pem"))
		if err != nil {
			t.Fatal(err)
		}
		ca, err := Load(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca-key.pem"))
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AddCert(ca.Certificate)
		leaf, _ := x509.ParseCertificate(pair.Certificate[0])
		for _, name := range names {
			if _, err = leaf.Verify(x509.VerifyOptions{DNSName: name, Roots: roots}); err != nil {
				t.Fatalf("%s not served: %v", name, err)
			}
		}
		if _, err = leaf.Verify(x509.VerifyOptions{DNSName: "prisme.mantara.fr", Roots: roots}); err == nil {
			t.Fatal("unexpected public name")
		}
		if !ca.Certificate.NotAfter.After(time.Now().AddDate(4, 11, 0)) {
			t.Fatal("CA validity too short")
		}
		return leaf
	}
	first := verify()
	expires, err := RenewServer(dir, names, 30)
	if err != nil {
		t.Fatal(err)
	}
	second := verify()
	if second.SerialNumber.Cmp(first.SerialNumber) == 0 || !second.NotAfter.Equal(expires) || expires.After(time.Now().AddDate(0, 0, 31)) {
		t.Fatal("server certificate not renewed as requested")
	}
	if current, _ := os.ReadFile(filepath.Join(dir, "ca-key.pem")); string(current) != string(caKey) {
		t.Fatal("renewal replaced the CA")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.new")); len(leftovers) != 0 {
		t.Fatal("temporary files left behind")
	}
}

func TestProductionPKIRejectsUnsafeRequests(t *testing.T) {
	for _, c := range []struct {
		names []string
		years int
		days  int
	}{{nil, 5, 397}, {[]string{"Prisme-API"}, 5, 397}, {[]string{"prisme api"}, 5, 397}, {[]string{"*.mantara.fr"}, 5, 397}, {[]string{"10.0.0.1."}, 5, 397}, {[]string{"prisme-api"}, 0, 397}, {[]string{"prisme-api"}, 5, 0}, {[]string{"prisme-api"}, 5, 900}, {[]string{"prisme-api"}, 1, 400}} {
		if err := InitProduction(filepath.Join(t.TempDir(), "pki"), "CA", c.names, c.years, c.days); err == nil {
			t.Fatalf("unsafe request accepted: %+v", c)
		}
	}
	if _, err := RenewServer(t.TempDir(), []string{"prisme-api"}, 30); err == nil {
		t.Fatal("renewal without a CA accepted")
	}
}
