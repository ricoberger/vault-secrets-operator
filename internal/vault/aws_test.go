package vault

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/vault/api"
)

// TestAWSEC2Nonce verifies that the reauthentication nonce is the deterministic
// hex encoded SHA-256 sum of the Kubernetes service account token.
func TestAWSEC2Nonce(t *testing.T) {
	kubeToken := []byte("a-kubernetes-service-account-token")
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, kubeToken, 0o600); err != nil {
		t.Fatalf("failed to write kube token file: %v", err)
	}

	nonce, err := awsEC2Nonce(tokenPath)
	if err != nil {
		t.Fatalf("awsEC2Nonce returned an error: %v", err)
	}

	want := fmt.Sprintf("%x", sha256.Sum256(kubeToken))
	if nonce != want {
		t.Errorf("nonce = %q, want %q", nonce, want)
	}
}

// TestAWSEC2NonceMissingFile verifies that a missing token file results in an
// error.
func TestAWSEC2NonceMissingFile(t *testing.T) {
	if _, err := awsEC2Nonce(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected an error for a missing token file, got nil")
	}
}

// TestExportAWSCredentials verifies that credentials resolved from the
// environment are re-exported so that the Vault AWS auth library can consume
// them.
func TestExportAWSCredentials(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_SESSION_TOKEN", "")

	if err := exportAWSCredentials(context.Background(), "eu-central-1"); err != nil {
		t.Fatalf("exportAWSCredentials returned an error: %v", err)
	}

	if got := os.Getenv("AWS_ACCESS_KEY_ID"); got != "AKIDEXAMPLE" {
		t.Errorf("AWS_ACCESS_KEY_ID = %q, want %q", got, "AKIDEXAMPLE")
	}
	if got := os.Getenv("AWS_SECRET_ACCESS_KEY"); got != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("AWS_SECRET_ACCESS_KEY mismatch, got %q", got)
	}
}

// TestAWSLoginIAM exercises the full IAM login path against a mock Vault server:
// credentials are resolved from the environment, the official library signs an
// sts:GetCallerIdentity request locally (no network call to AWS), and the login
// data is posted to Vault. This is the regression guard for issue #364.
func TestAWSLoginIAM(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_SESSION_TOKEN", "")

	var (
		gotPath string
		gotBody map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"auth":{"client_token":"s.test-token","lease_duration":3600,"renewable":true}}`))
	}))
	defer srv.Close()

	cfg := api.DefaultConfig()
	cfg.Address = srv.URL
	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	secret, err := awsLogin(context.Background(), client, awsAuthConfig{
		authType:       "iam",
		region:         "eu-central-1",
		role:           "my-vault-role",
		mountPath:      "aws",
		serverIDHeader: "vault.example.com",
	})
	if err != nil {
		t.Fatalf("awsLogin returned an error: %v", err)
	}

	if secret.Auth == nil || secret.Auth.ClientToken != "s.test-token" {
		t.Fatalf("unexpected secret auth: %+v", secret.Auth)
	}
	if gotPath != "/v1/auth/aws/login" {
		t.Errorf("login path = %q, want /v1/auth/aws/login", gotPath)
	}
	if got := gotBody["role"]; got != "my-vault-role" {
		t.Errorf("role = %v, want my-vault-role", got)
	}
	if got := gotBody["iam_http_request_method"]; got != http.MethodPost {
		t.Errorf("iam_http_request_method = %v, want POST", got)
	}

	// The signed request headers must contain the Vault server ID header.
	headersField, ok := gotBody["iam_request_headers"].(string)
	if !ok {
		t.Fatalf("iam_request_headers has type %T, want base64 string", gotBody["iam_request_headers"])
	}
	headersJSON, err := base64.StdEncoding.DecodeString(headersField)
	if err != nil {
		t.Fatalf("iam_request_headers is not valid base64: %v", err)
	}
	var rawHeaders map[string][]string
	if err := json.Unmarshal(headersJSON, &rawHeaders); err != nil {
		t.Fatalf("failed to unmarshal iam_request_headers: %v", err)
	}
	if got := http.Header(rawHeaders).Get("X-Vault-AWS-IAM-Server-ID"); got != "vault.example.com" {
		t.Errorf("X-Vault-AWS-IAM-Server-ID = %q, want vault.example.com", got)
	}

	// The request must be signed (SigV4) and scoped to the configured region.
	auth := http.Header(rawHeaders).Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
		t.Fatalf("Authorization = %q, want it to start with AWS4-HMAC-SHA256", auth)
	}
	if !strings.Contains(auth, "/eu-central-1/sts/aws4_request") {
		t.Errorf("Authorization credential scope = %q, want it to contain /eu-central-1/sts/aws4_request", auth)
	}

	// The signed URL must point at the (global) STS endpoint, matching the
	// behaviour of the official library.
	urlField, _ := gotBody["iam_request_url"].(string)
	rawURL, err := base64.StdEncoding.DecodeString(urlField)
	if err != nil {
		t.Fatalf("iam_request_url is not valid base64: %v", err)
	}
	if want := "sts.amazonaws.com"; !strings.Contains(string(rawURL), want) {
		t.Errorf("iam_request_url = %q, want it to contain %q", string(rawURL), want)
	}
}

// TestAWSLoginInvalidType verifies that an unknown auth type returns an error.
func TestAWSLoginInvalidType(t *testing.T) {
	_, err := awsLogin(context.Background(), nil, awsAuthConfig{authType: "invalid"})
	if err == nil {
		t.Fatal("expected an error for an invalid auth type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid aws authentication type") {
		t.Errorf("unexpected error: %v", err)
	}
}
