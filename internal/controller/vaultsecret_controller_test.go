package controller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// testCertPEM generates a self-signed certificate which expires at notAfter and
// returns it as a PEM encoded byte slice.
func testCertPEM(t *testing.T, commonName string, notAfter time.Time) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// TestCertificateExpiration verifies that certificateExpiration extracts the
// expiration date of the certificate which expires first from the Secret data.
func TestCertificateExpiration(t *testing.T) {
	leaf := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	ca := time.Now().Add(720 * time.Hour).Truncate(time.Second)

	leafPEM := testCertPEM(t, "leaf", leaf)
	caPEM := testCertPEM(t, "ca", ca)

	t.Run("single certificate", func(t *testing.T) {
		got, ok := certificateExpiration(map[string][]byte{
			"certificate": leafPEM,
		})
		if !ok {
			t.Fatal("expected a certificate to be found")
		}
		if !got.Equal(leaf) {
			t.Errorf("expiration = %s, want %s", got, leaf)
		}
	})

	t.Run("leaf and ca in separate keys returns earliest", func(t *testing.T) {
		got, ok := certificateExpiration(map[string][]byte{
			"certificate": leafPEM,
			"issuing_ca":  caPEM,
			"private_key": []byte("not a certificate"),
		})
		if !ok {
			t.Fatal("expected a certificate to be found")
		}
		if !got.Equal(leaf) {
			t.Errorf("expiration = %s, want %s (the leaf)", got, leaf)
		}
	})

	t.Run("certificate chain in a single value returns earliest", func(t *testing.T) {
		chain := append(append([]byte{}, caPEM...), leafPEM...)
		got, ok := certificateExpiration(map[string][]byte{
			"tls.crt": chain,
		})
		if !ok {
			t.Fatal("expected a certificate to be found")
		}
		if !got.Equal(leaf) {
			t.Errorf("expiration = %s, want %s (the leaf)", got, leaf)
		}
	})

	t.Run("certificate moved to a custom key via template", func(t *testing.T) {
		got, ok := certificateExpiration(map[string][]byte{
			"my-custom-key": leafPEM,
		})
		if !ok {
			t.Fatal("expected a certificate to be found")
		}
		if !got.Equal(leaf) {
			t.Errorf("expiration = %s, want %s", got, leaf)
		}
	})

	t.Run("no certificate present", func(t *testing.T) {
		if _, ok := certificateExpiration(map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("s3cr3t"),
		}); ok {
			t.Error("expected no certificate to be found")
		}
	})

	t.Run("private key only", func(t *testing.T) {
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("dummy")})
		if _, ok := certificateExpiration(map[string][]byte{
			"private_key": keyPEM,
		}); ok {
			t.Error("expected no certificate to be found")
		}
	})

	t.Run("empty data", func(t *testing.T) {
		if _, ok := certificateExpiration(map[string][]byte{}); ok {
			t.Error("expected no certificate to be found")
		}
	})
}
