package vault

import (
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
)

// RequestToken is a function to request a new Vault token, specific for auth method.
type RequestToken func(*Client) error

// Client is the structure of our global client for Vault.
type Client struct {
	// client is the API client for requests against Vault.
	client *api.Client
	// tokenLeaseDuration is the lease duration of the token for the interaction with vault.
	tokenLeaseDuration int
	// tokenRenewalInterval is the time between two successive vault token renewals.
	tokenRenewalInterval float64
	// tokenRenewalRetryInterval is the time until a failed vault token renewal is retried.
	tokenRenewalRetryInterval float64
	// tokenMaxTTL is the maximum lifetime for the token in seconds, after that time a new token
	// must be requested. Zero means the tokens lives and can be renewed forever.
	tokenMaxTTL int
	// requestToken is a function to request a new Vault token, specific for auth method.
	requestToken RequestToken
	// vault namespace
	rootVaultNamespace string
}

// RenewToken renews the provided token after the half of the lease duration is
// passed, retrying every 30 seconds in case of errors.
func (c *Client) RenewToken() {
	started := time.Now()
	for {
		// Set the namespace to the value from the VAULT_NAMESPACE environment
		// variable, because the namespace will always change, when a secret is
		// requested.
		if c.rootVaultNamespace != "" {
			c.client.SetNamespace(c.rootVaultNamespace)
		}

		// Request a new token if the actual token lifetime more than the specified maximum
		// lifetime.
		elapsed := time.Now().Sub(started).Seconds()
		if c.tokenMaxTTL > 0 && elapsed >= float64(c.tokenMaxTTL) && c.requestToken != nil {
			log.Info("Request new Vault token")
			err := c.requestToken(c)
			if err != nil {
				log.Error(err, "Could not request a new token")
				time.Sleep(time.Duration(c.tokenRenewalRetryInterval) * time.Second)
			} else {
				started = time.Now()
				time.Sleep(time.Duration(c.tokenRenewalInterval) * time.Second)
			}
			continue
		}

		log.Info("Renew Vault token")
		_, err := c.client.Auth().Token().RenewSelf(c.tokenLeaseDuration)
		if err != nil {
			log.Error(err, "Could not renew token")

			lookup, err := c.client.Auth().Token().LookupSelf()
			if err != nil {
				log.WithValues("error", err.Error()).Info("LookupSelf failed")
			} else {
				log.Info(fmt.Sprintf("Token information: %#v", lookup))
			}

			time.Sleep(time.Duration(c.tokenRenewalRetryInterval) * time.Second)
		} else {
			time.Sleep(time.Duration(c.tokenRenewalInterval) * time.Second)
		}
	}
}

// GetSecret returns the value for a given secret.
func (c *Client) GetSecret(secretEngine string, path string, keys []string, version int, isBinary bool, vaultNamespace string) (map[string][]byte, error) {
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
	// secrets path. If a version is provided we fill the request data with the
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

	// The structure for a KVv2 secret differs from the structure of a KV1
	// secret. Next to the secret 'data' a KVv2 secret contains also some
	// 'metadata'. We only need the 'data' field to go on.
	secretData := secret.Data
	if v2 {
		var ok bool
		secretData, ok = secret.Data["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("could not parse secret")
		}
	}

	// Convert the secret data for a Kubernetes secret. We only add the provided
	// keys to the resulting data or if there are no keys provided we add all
	// keys of the secret.
	// To support nested secret values we check the type of the value first. If
	// The type is 'map[string]interface{}' we marshal the value to a JSON
	// string, which can be used for the Kubernetes secret.
	data := make(map[string][]byte)
	for key, value := range secretData {
		if len(keys) == 0 || contains(key, keys) {
			switch value.(type) {
			case map[string]interface{}:
				jsonString, err := json.Marshal(value)
				if err != nil {
					return nil, err
				}
				data[key] = []byte(jsonString)
			case string:
				if isBinary {
					data[key], err = b64.StdEncoding.DecodeString(value.(string))
					if err != nil {
						return nil, err
					}
				} else {
					data[key] = []byte(value.(string))
				}
			case json.Number:
				data[key] = []byte(value.(json.Number))
			case bool:
				data[key] = []byte(fmt.Sprintf("%t", value.(bool)))
			default:
				return nil, fmt.Errorf("could not parse secret value")
			}
		}
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

// contains checks if a given key is in a slice of keys.
func contains(key string, keys []string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}

	return false
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
