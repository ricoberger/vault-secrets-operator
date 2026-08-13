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

	ricobergerdev1alpha1 "github.com/ricoberger/vault-secrets-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
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

// TestMergeSecretsPaths verifies that merging multiple Vault paths follows the
// first-wins strategy for duplicate keys.
func TestMergeSecretsPaths(t *testing.T) {
	secretsPaths := []secretPath{
		{Path: "kv/secret1", Secrets: map[string][]byte{
			"shared": []byte("from-secret1"),
			"only1":  []byte("value1"),
		}},
		{Path: "kv/secret2", Secrets: map[string][]byte{
			"shared": []byte("from-secret2"),
			"only2":  []byte("value2"),
		}},
	}

	got := mergeSecretsPaths(secretsPaths)

	want := map[string]string{
		"shared": "from-secret1",
		"only1":  "value1",
		"only2":  "value2",
	}

	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if string(got[k]) != v {
			t.Errorf("key %q = %q, want %q", k, string(got[k]), v)
		}
	}
}

// TestNewSecretForCRMultiplePaths verifies that the merged data and the
// per-path templating context (.SecretsPaths) are set up correctly when a
// secret is built from multiple Vault paths.
func TestNewSecretForCRMultiplePaths(t *testing.T) {
	cr := &ricobergerdev1alpha1.VaultSecret{}
	cr.Name = "example"
	cr.Namespace = "default"
	cr.Spec.Path = "kv/secret1"
	cr.Spec.Paths = []string{"kv/secret2"}
	cr.Spec.Type = corev1.SecretTypeOpaque

	secretsPaths := []secretPath{
		{Path: "kv/secret1", Secrets: map[string][]byte{"shared": []byte("first")}},
		{Path: "kv/secret2", Secrets: map[string][]byte{"shared": []byte("second")}},
	}
	data := mergeSecretsPaths(secretsPaths)

	t.Run("without templates uses first-wins merged data", func(t *testing.T) {
		secret, err := newSecretForCR(cr, data, secretsPaths)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(secret.Data["shared"]); got != "first" {
			t.Errorf("shared = %q, want %q", got, "first")
		}
	})

	t.Run("templates expose all paths via SecretsPaths", func(t *testing.T) {
		crWithTemplates := cr.DeepCopy()
		crWithTemplates.Spec.Templates = map[string]string{
			"combined": "{%- range .SecretsPaths %}{% .Secrets.shared %},{% end -%}",
			"firstWin": "{% .Secrets.shared %}",
		}

		secret, err := newSecretForCR(crWithTemplates, data, secretsPaths)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(secret.Data["combined"]); got != "first,second," {
			t.Errorf("combined = %q, want %q", got, "first,second,")
		}
		if got := string(secret.Data["firstWin"]); got != "first" {
			t.Errorf("firstWin = %q, want %q", got, "first")
		}
	})
}
