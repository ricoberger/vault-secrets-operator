package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VaultSecretSpec defines the desired state of VaultSecret
// +k8s:openapi-gen=true
type VaultSecretSpec struct {
	// ReconcileStrategy defines the strategy for reconcilation. The default value is "Replace", which replaces any
	// existing data keys in a secret with the loaded keys from Vault. The second valid value is "Merge" wiche merges
	// the loaded keys from Vault with the existing keys in a secret. Duplicated keys will be replaced with the value
	// from Vault. Other values are not valid for this field.
	ReconcileStrategy string `json:"reconcileStrategy,omitempty"`
	// Keys is an array of Keys, which should be included in the Kubernetes
	// secret. If the Keys field is ommitted all keys from the Vault secret will
	// be included in the Kubernetes secret.
	Keys []string `json:"keys,omitempty"`
	// Path is the path of the corresponding secret in Vault.
	Path string `json:"path"`
	// SecretEngine specifies the type of the Vault secret engine in which the
	// secret is stored. Currently the 'KV Secrets Engine - Version 1' and
	// 'KV Secrets Engine - Version 2' are supported. The value must be 'kv'. If
	// the value is omitted or an other values is used the Vault Secrets
	// Operator will try to use the KV secret engine.
	SecretEngine string `json:"secretEngine,omitempty"`
	// Type is the type of the Kubernetes secret, which will be created by the
	// Vault Secrets Operator.
	Type corev1.SecretType `json:"type"`
	// Version sets the version of the secret which should be used. The version
	// is only used if the KVv2 secret engine is used. If the version is
	// omitted the Operator uses the latest version of the secret. If the version
	// omitted and the VAULT_RECONCILIATION_TIME environment variable is set, the
	// Kubernetes secret will be updated if the Vault secret changes.
	Version int `json:"version,omitempty"`
	// isBinary is a flag indicates if data stored in vault is
	// binary data. Since vault does not store binary data natively,
	// the binary data is stored as base64 encoded. However, same data get encoded
	// again when operator stored them as secret in k8s which caused the data to
	// get double encoded. This flag will skip the base64 encode which is needed
	// for string data to avoid the double encode problem.
	IsBinary bool `json:"isBinary,omitempty"`
}

// VaultSecretStatus defines the observed state of VaultSecret
// +k8s:openapi-gen=true
type VaultSecretStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VaultSecret is the Schema for the vaultsecrets API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type VaultSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VaultSecretSpec   `json:"spec,omitempty"`
	Status VaultSecretStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VaultSecretList contains a list of VaultSecret
type VaultSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VaultSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VaultSecret{}, &VaultSecretList{})
}
