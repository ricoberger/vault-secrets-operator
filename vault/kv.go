package vault

import (
	"errors"
	"fmt"
	"github.com/hashicorp/vault/api"
	"path"
	"strconv"
	"strings"
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

// kvPreflightVersionRequest checks which version of the key values secrets
// engine is used for the given path.
// This function is copy/past from the github.com/hashicorp/vault repository,
// see: https://github.com/hashicorp/vault/blob/f843c09dd15ca4982e60fa12dea48c8f7d7e0373/command/kv_helpers.go#L44
func (c *Client) kvPreflightVersionRequest(path string) (string, int, error) {
	// We don't want to use a wrapping call here so save any custom value and
	// restore after
	currentWrappingLookupFunc := c.client.CurrentWrappingLookupFunc()
	c.client.SetWrappingLookupFunc(nil)
	defer c.client.SetWrappingLookupFunc(currentWrappingLookupFunc)
	currentOutputCurlString := c.client.OutputCurlString()
	c.client.SetOutputCurlString(false)
	defer c.client.SetOutputCurlString(currentOutputCurlString)

	r := c.client.NewRequest("GET", "/v1/sys/internal/ui/mounts/"+path)
	resp, err := c.client.RawRequest(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		// If we get a 404 we are using an older version of vault, default to
		// version 1
		if resp != nil && resp.StatusCode == 404 {
			return "", 1, nil
		}

		return "", 0, err
	}

	secret, err := api.ParseSecret(resp.Body)
	if err != nil {
		return "", 0, err
	}
	if secret == nil {
		return "", 0, errors.New("nil response from pre-flight request")
	}
	var mountPath string
	if mountPathRaw, ok := secret.Data["path"]; ok {
		mountPath = mountPathRaw.(string)
	}
	options := secret.Data["options"]
	if options == nil {
		return mountPath, 1, nil
	}
	versionRaw := options.(map[string]interface{})["version"]
	if versionRaw == nil {
		return mountPath, 1, nil
	}
	version := versionRaw.(string)
	switch version {
	case "", "1":
		return mountPath, 1, nil
	case "2":
		return mountPath, 2, nil
	}

	return mountPath, 1, nil
}

// isKVv2 returns true if a KVv2 is used for the given path and false if a KVv1
// secret engine is used.
// This function is copy/past from the github.com/hashicorp/vault repository,
// see: https://github.com/hashicorp/vault/blob/f843c09dd15ca4982e60fa12dea48c8f7d7e0373/command/kv_helpers.go#L99
func (c *Client) isKVv2(path string) (string, bool, error) {
	mountPath, version, err := c.kvPreflightVersionRequest(path)
	if err != nil {
		return "", false, err
	}

	return mountPath, version == 2, nil
}

// addPrefixToVKVPath adds the given prefix to the given path.
// This function is copy/past from the github.com/hashicorp/vault repository,
// see: https://github.com/hashicorp/vault/blob/f843c09dd15ca4982e60fa12dea48c8f7d7e0373/command/kv_helpers.go#L108
func (c *Client) addPrefixToVKVPath(p, mountPath, apiPrefix string) string {
	switch {
	case p == mountPath, p == strings.TrimSuffix(mountPath, "/"):
		return path.Join(mountPath, apiPrefix)
	default:
		p = strings.TrimPrefix(p, mountPath)
		return path.Join(mountPath, apiPrefix, p)
	}
}
