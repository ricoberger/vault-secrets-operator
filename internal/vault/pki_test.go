package vault

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/vault/api"
)

// newTestClient returns a Client whose underlying Vault API client talks to the
// given test server.
func newTestClient(t *testing.T, url string) *Client {
	t.Helper()

	cfg := api.DefaultConfig()
	cfg.Address = url
	apiClient, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	return &Client{client: apiClient}
}

// TestGetCertificateCAChain verifies that the "ca_chain" field returned by
// Vault's PKI issue endpoint (a list of PEM encoded certificates) is joined
// into a single PEM string and exposed alongside the other certificate fields.
func TestGetCertificateCAChain(t *testing.T) {
	const (
		intermediate = "-----BEGIN CERTIFICATE-----\nintermediate\n-----END CERTIFICATE-----"
		root         = "-----BEGIN CERTIFICATE-----\nroot\n-----END CERTIFICATE-----"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"certificate": "-----BEGIN CERTIFICATE-----\ncert\n-----END CERTIFICATE-----",
				"expiration": 1649769202,
				"issuing_ca": "` + escape(intermediate) + `",
				"ca_chain": ["` + escape(intermediate) + `", "` + escape(root) + `"],
				"private_key": "-----BEGIN RSA PRIVATE KEY-----\nkey\n-----END RSA PRIVATE KEY-----",
				"private_key_type": "rsa",
				"serial_number": "00:11:22"
			}
		}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	data, expiration, err := client.GetCertificate("pki", "example-dot-com", nil)
	if err != nil {
		t.Fatalf("GetCertificate returned an error: %v", err)
	}

	if expiration == nil || expiration.Unix() != 1649769202 {
		t.Errorf("unexpected expiration: %v", expiration)
	}

	want := intermediate + "\n" + root
	if got := string(data["ca_chain"]); got != want {
		t.Errorf("ca_chain = %q, want %q", got, want)
	}

	if got := string(data["issuing_ca"]); got != intermediate {
		t.Errorf("issuing_ca = %q, want %q", got, intermediate)
	}
}

// escape turns the newlines of a PEM string into the escaped form used inside a
// JSON string literal.
func escape(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' {
			out = append(out, '\\', 'n')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
