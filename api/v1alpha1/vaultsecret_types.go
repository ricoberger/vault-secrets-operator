package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JksSpec defines the JKS, and whether to create or not.
type JksSpec struct {
	// Type of Jks, could be 'keystore' or 'truststore'. This would also dictate what's in vault secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=keystore;truststore
	Type string `json:"type"`
	// Name of keystore/truststore. Eg. keystore.jks, truststore.jks, ...
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// VaultSecretSpec defines the desired state of VaultSecret
type VaultSecretSpec struct {
	// VaultRole can be used to specify the Vault role, which should be used to get the secret from Vault. If the
	// vaultRole property is set a new client with the specified Vault Role will be created and the shared client is
	// ignored. If the operator is configured using the token auth method this property has no effect.
	VaultRole string `json:"vaultRole,omitempty"`
	// VaultNamespace can be used to specify the Vault namespace for a secret. When this value is set, the
	// X-Vault-Namespace header will be set for the request. More information regarding namespaces can be found in the
	// Vault Enterprise documentation: https://www.vaultproject.io/docs/enterprise/namespaces
	VaultNamespace string `json:"vaultNamespace,omitempty"`
	// ReconcileStrategy defines the strategy for reconcilation. The default value is "Replace", which replaces any
	// existing data keys in a secret with the loaded keys from Vault. The second valid value is "Merge" wiche merges
	// the loaded keys from Vault with the existing keys in a secret. Duplicated keys will be replaced with the value
	// from Vault. Other values are not valid for this field.
	ReconcileStrategy string `json:"reconcileStrategy,omitempty"`
	// Keys is an array of Keys, which should be included in the Kubernetes
	// secret. If the Keys field is ommitted all keys from the Vault secret will
	// be included in the Kubernetes secret.
	Keys []string `json:"keys,omitempty"`
	// Templates, if not empty will be run through the the Go templating engine, with `.Secrets` being mapped
	// to the list of secrets received from Vault. When omitted set, all secrets will be added as key/val pairs
	// under Secret.data.
	Templates map[string]string `json:"templates,omitempty"`
	// Path is the path of the corresponding secret in Vault.
	// +kubebuilder:validation:Optional
	Path string `json:"path",omitempty`
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
	// Indicates whether or not the k8s secret that will be created
	// will create a keystore/truststore key off of the keys in the vault path.
	Jks JksSpec `json:"jks,omitempty"`
}

// VaultSecretStatus defines the observed state of VaultSecret
type VaultSecretStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// VaultSecret is the Schema for the vaultsecrets API
type VaultSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VaultSecretSpec   `json:"spec,omitempty"`
	Status VaultSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VaultSecretList contains a list of VaultSecret
type VaultSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VaultSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VaultSecret{}, &VaultSecretList{})
}
