package validators

import (
	"testing"

	ricobergerdev1alpha1 "github.com/ricoberger/vault-secrets-operator/api/v1alpha1"
)

func TestValidatePaths(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		paths   []string
		wantErr bool
	}{
		{name: "only path", path: "kv/secret", wantErr: false},
		{name: "only paths", paths: []string{"kv/secret"}, wantErr: false},
		{name: "path and paths", path: "kv/secret1", paths: []string{"kv/secret2"}, wantErr: false},
		{name: "neither set", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &ricobergerdev1alpha1.VaultSecret{}
			instance.Spec.Path = tt.path
			instance.Spec.Paths = tt.paths

			err := ValidatePaths(instance)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePaths() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePKIWithPaths(t *testing.T) {
	instance := &ricobergerdev1alpha1.VaultSecret{}
	instance.Spec.SecretEngine = "pki"
	instance.Spec.Role = "example"
	instance.Spec.EngineOptions = map[string]string{"common_name": "example.com"}
	instance.Spec.Paths = []string{"pki/issue"}

	if err := ValidatePKI(instance); err == nil {
		t.Error("ValidatePKI() expected an error when 'paths' is set for the pki engine")
	}
}
