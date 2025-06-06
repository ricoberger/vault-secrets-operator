package vault

import (
	"fmt"
	"github.com/hashicorp/vault/api"
	"strconv"
)

// GetSecret returns the value for a given secret.
func (c *Client) GetSecret(path string, version int, vaultNamespace string) (*api.Secret, error) {
	// Get the secret for the given path and return the secret data.
	log.Info(fmt.Sprintf("Read secret %s", path))

	// Check if the vaultNamespace field is set for the secret. If the field is
	// set we use the configured root namespace from the VAULT_NAMESPACE and
	// the value from the vaultNamespace field to build the final namespace
	// path. If the field is not set but VAULT_NAMESPACE has a value, we
	// just use the latter.
	// If the vaultNamespace field is set, but not the VAULT_NAMESPACE
	// environment variable we return an error, because the authentication
	// already failed.
	if c.rootVaultNamespace != "" {
		log.WithValues("rootVaultNamespace", c.rootVaultNamespace, "vaultNamespace", vaultNamespace).Info(fmt.Sprintf("Use Vault Namespace to read secret %s", path))
		if vaultNamespace != "" {
			c.client.SetNamespace(c.rootVaultNamespace + "/" + vaultNamespace)
		} else {
			c.client.SetNamespace(c.rootVaultNamespace)
		}
	} else if c.rootVaultNamespace == "" && vaultNamespace != "" {
		return nil, fmt.Errorf("vaultNamespace field can not be used, because the VAULT_NAMESPACE environment variable is not set")
	}

	// Check if the KVv1 or KVv2 is used for the provided secret and determin
	// the mount path of the secrets engine.
	mountPath, v2, err := c.isKVv2(path)
	if err != nil {
		return nil, err
	}

	// If the KVv2 secrets engine is used we add the 'data' prefix to the
	// secret's path. If a version is provided we fill the request data with the
	// version parameter.
	// NOTE: Without any request data the ReadWithData method will act like the
	// Read method.
	reqData := make(map[string][]string)

	if v2 {
		path = c.addPrefixToVKVPath(path, mountPath, "data")

		if version > 0 {
			reqData["version"] = []string{strconv.Itoa(version)}
		}
	}

	secret, err := c.client.Logical().ReadWithData(path, reqData)
	if err != nil {
		return nil, err
	}

	if secret == nil {
		return nil, fmt.Errorf("secret is nil")
	}

	return secret, nil
}

func (c *Client) KVRenderData(secret *api.Secret, keys []string, isBinary bool) (map[string][]byte, error) {
	// The structure for a KVv2 secret differs from the structure of a KV1
	// secret. Next to the secret 'data' a KVv2 secret contains also some
	// 'metadata'. We only need the 'data' field to go on.
	var secretData map[string]interface{}
	var ok bool
	secretData, ok = secret.Data["data"].(map[string]interface{})
	if !ok {
		secretData = secret.Data
	}

	data, err := convertData(secretData, keys, isBinary)
	if err != nil {
		return nil, err
	}

	// If the data map is empty we return an error. This can happend, if the
	// secret which was retrieved from Vault is under a KVv2 secrets engine, but
	// the secret engine was not provided in the cr for the secret. Then the
	// returned secret looks like this: &api.Secret{RequestID:\"be7b671f-a097-1081-15ec-b4710f2a6249\", LeaseID:\"\", LeaseDuration:0, Renewable:false, Data:map[string]interface {}(nil), Warnings:[]string{\"Invalid path for a versioned K/V secrets engine. See the API docs for the appropriate API endpoints to use. If using the Vault CLI, use 'vault kv get' for this operation.\"}, Auth:(*api.SecretAuth)(nil), WrapInfo:(*api.SecretWrapInfo)(nil)}"}
	if len(data) == 0 {
		return nil, fmt.Errorf("invalid secret data")
	}

	return data, nil
}
