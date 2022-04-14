package controllers

import (
	"fmt"
	ricobergerdev1alpha1 "github.com/ricoberger/vault-secrets-operator/api/v1alpha1"
)

func ValidatePKI(instance *ricobergerdev1alpha1.VaultSecret) error {
	if instance.Spec.SecretEngine != ricobergerdev1alpha1.PKIEngine {
		return fmt.Errorf("cannot validate non-PKI resource")
	}

	if instance.Spec.Role == "" {
		return fmt.Errorf("`Role' must be set")
	}

	if _, ok := instance.Spec.EngineOptions["common_name"]; !ok {
		return fmt.Errorf("`engineOptions.common_name' must be set")
	}

	return nil
}

func ValidateDatabase(instance *ricobergerdev1alpha1.VaultSecret) error {
	if instance.Spec.SecretEngine != "database" {
		return fmt.Errorf("cannot validate non-Database resource")
	}

	if instance.Spec.Role == "" {
		return fmt.Errorf("`Role' must be set")
	}

	return nil
}
