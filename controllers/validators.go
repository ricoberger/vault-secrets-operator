package controllers

import (
	"fmt"
	ricobergerdev1alpha1 "github.com/ricoberger/vault-secrets-operator/api/v1alpha1"
)

func ValidatePKI(instance *ricobergerdev1alpha1.VaultSecret) error {
	if instance.Spec.SecretEngine != "pki" {
		return nil
	}

	if instance.Spec.PKIRole == "" {
		return fmt.Errorf("`PKIRole' must be set")
	}

	if _, ok := instance.Spec.EngineOptions["common_name"]; !ok {
		return fmt.Errorf("`engineOptions.common_name' must be set")
	}

	return nil
}
