package validators

import (
	"fmt"

	ricobergerdev1alpha1 "github.com/ricoberger/vault-secrets-operator/api/v1alpha1"
)

func ValidatePKI(instance *ricobergerdev1alpha1.VaultSecret) error {
	if instance.Spec.SecretEngine != "pki" {
		return nil
	}

	if len(instance.Spec.Paths) > 0 {
		return fmt.Errorf("'paths' is not supported for the 'pki' secret engine")
	}

	if instance.Spec.Role == "" {
		return fmt.Errorf("'Role' must be set")
	}

	if _, ok := instance.Spec.EngineOptions["common_name"]; !ok {
		return fmt.Errorf("'engineOptions.common_name' must be set")
	}

	return nil
}

// ValidatePaths ensures that at least one Vault path is configured via the
// 'path' or 'paths' field.
func ValidatePaths(instance *ricobergerdev1alpha1.VaultSecret) error {
	if instance.Spec.Path == "" && len(instance.Spec.Paths) == 0 {
		return fmt.Errorf("at least one of 'path' or 'paths' must be set")
	}

	return nil
}
