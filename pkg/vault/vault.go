package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	// ErrMissingVaultAddress is our error, if the Vault address is missing.
	ErrMissingVaultAddress = errors.New("missing vault address")
	// ErrInvalidAuthMethod is our error, if a invalid authentication method is provided.
	ErrInvalidAuthMethod = errors.New("invalid auth method")
	// ErrMissingVaultToken is our error, if the Vault token is missing.
	ErrMissingVaultToken = errors.New("missing vault token")
	// ErrMissingVaultTokenLeaseDuration is our error, if the lease duration for the Vault token is missing.
	ErrMissingVaultTokenLeaseDuration = errors.New("missing lease duration for vault token")
	// ErrMissingVaultKubernetesPath is our error, if the mount path of the Kubernetes Auth Method is not provided.
	ErrMissingVaultKubernetesPath = errors.New("missing ttl for vault token")
	// ErrMissingVaultKubernetesRole is our error, if the role for the Kubernetes Auth Method is not provided.
	ErrMissingVaultKubernetesRole = errors.New("missing ttl for vault token")
	// ErrMissingVaultAuthInfo is our error, if sth. went wrong during the authentication agains Vault.
	ErrMissingVaultAuthInfo = errors.New("missing authentication information")
	// ErrInvalidPath is our error, if the path of a secret is invalid.
	ErrInvalidPath = errors.New("invalid path")
	// ErrSecretIsNil is our error, if the returned secret from Vault is nil.
	ErrSecretIsNil = errors.New("secret is nil")
	// ErrParseSecret is our error if the secret could not be parsed.
	ErrParseSecret = errors.New("could not parse secret")
	// ErrInvalidSecretData is our error if the returned secret data is invalid.
	ErrInvalidSecretData = errors.New("invalid secret data")
	// ErrParseSecretValue is our error if the returned secret data is invalid.
	ErrParseSecretValue = errors.New("could not parse secret value")

	// log is our customized logger.
	log = logf.Log.WithName("vault")

	// client is the API client for the interaction with the Vault API.
	client *api.Client

	// tokenLeaseDuration is the lease duration of the token for the interaction with vault.
	tokenLeaseDuration int
)

// CreateClient creates a new Vault API client.
func CreateClient() error {
	var err error
	vaultAddress := os.Getenv("VAULT_ADDRESS")
	vaultAuthMethod := os.Getenv("VAULT_AUTH_METHOD")
	vaultToken := os.Getenv("VAULT_TOKEN")
	vaultTokenLeaseDuration := os.Getenv("VAULT_TOKEN_LEASE_DURATION")
	vaultKubernetesPath := os.Getenv("VAULT_KUBERNETES_PATH")
	vaultKubernetesRole := os.Getenv("VAULT_KUBERNETES_ROLE")

	// Validate that the Vault address is set.
	if vaultAddress == "" {
		return ErrMissingVaultAddress
	}

	// Create new Vault configuration. This configuration is used to create the
	// API client. We set the timeout of the HTTP client to 10 seconds.
	// See: https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
	config := &api.Config{
		Address: vaultAddress,
		HttpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	client, err = api.NewClient(config)
	if err != nil {
		return err
	}

	// Check which authentication method should be used.
	if vaultAuthMethod == "token" {
		// Check the required token and the provided lease duration for the
		// token. If the values are empty or the lease duration could not be
		// parsed we return an error.
		if vaultToken == "" {
			return ErrMissingVaultToken
		}

		if vaultTokenLeaseDuration == "" {
			return ErrMissingVaultToken
		}

		if tokenLeaseDuration, err = strconv.Atoi(vaultTokenLeaseDuration); err != nil {
			return err
		}

		// Set the token, which should be used for the interaction with Vault.
		client.SetToken(vaultToken)
	} else if vaultAuthMethod == "kubernetes" {
		// Check the required mount path and role for the Kubernetes Auth
		// Method. If one of the env variable is missing we return an error.
		if vaultKubernetesPath == "" {
			return ErrMissingVaultKubernetesPath
		}

		if vaultKubernetesRole == "" {
			return ErrMissingVaultKubernetesRole
		}

		// Read the service account token value and create a map for the
		// authentication against Vault.
		kubeToken, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
		if err != nil {
			return err
		}

		data := make(map[string]interface{})
		data["jwt"] = string(kubeToken)
		data["role"] = vaultKubernetesRole

		// Authenticate against vault using the Kubernetes Auth Method and set
		// the token which the client should use for further interactions with
		// Vault. We also set the lease duration of the token for the renew
		// function.
		secret, err := client.Logical().Write(vaultKubernetesPath+"/login", data)
		if err != nil {
			return err
		} else if secret.Auth == nil {
			return ErrMissingVaultAuthInfo
		}

		tokenLeaseDuration = secret.Auth.LeaseDuration
		client.SetToken(secret.Auth.ClientToken)
	} else {
		return ErrInvalidAuthMethod
	}

	return nil
}

// RenewToken renews the provided token after the half of the lease duration is
// passed.
func RenewToken() {
	for {
		log.Info("Renew Vault token")

		_, err := client.Auth().Token().RenewSelf(tokenLeaseDuration)
		if err != nil {
			log.Error(err, "Could not renew token")
		}

		time.Sleep(time.Duration(float64(tokenLeaseDuration)*0.5) * time.Second)
	}
}

// GetSecret returns the value for a given secret.
func GetSecret(secretEngine string, path string, keys []string, version int) (map[string][]byte, error) {
	// Get the secret for the given path and return the secret data.
	log.Info(fmt.Sprintf("Read secret %s", path))

	// Check if the 'KV Secrets Engine - Version 1' is used for the provided
	// secret. If the secret is stored in a KV2 secrets engine the path must
	// contain a 'data' at the second position. If the path does not contain the
	// 'data' part we add it.
	if secretEngine == "kv2" {
		pathParts := strings.Split(path, "/")
		if len(pathParts) < 2 {
			return nil, ErrInvalidPath
		}

		if pathParts[1] != "data" {
			path = pathParts[0] + "/data/" + strings.Join(pathParts[1:], "/")
		}
	}

	// Check if the secret engine is KV2. If yes, we also check if a version
	// is provided (when not the version will be 0) and fill the request data
	// with the version parameter. If the version is omitted or KV1 secret
	// engine is used the ReadWithData acts like the Read method.
	reqData := make(map[string][]string)

	if secretEngine == "kv2" && version != 0 {
		reqData["version"] = []string{strconv.Itoa(version)}
	}

	secret, err := client.Logical().ReadWithData(path, reqData)
	if err != nil {
		return nil, err
	}

	if secret == nil {
		return nil, ErrSecretIsNil
	}

	// The structure for a KV2 secret differs from the structure of a KV1
	// secret. Next to the secret 'data' a KV2 secret contains also some
	// 'metadata'. We only need the 'data' field to go on.
	secretData := secret.Data
	if secretEngine == "kv2" {
		var ok bool
		secretData, ok = secret.Data["data"].(map[string]interface{})
		if !ok {
			return nil, ErrParseSecret
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
				data[key] = []byte(value.(string))
			case json.Number:
				data[key] = []byte(value.(json.Number))
			case bool:
				data[key] = []byte(fmt.Sprintf("%t", value.(bool)))
			default:
				return nil, ErrParseSecretValue
			}
		}
	}

	// If the data map is empty we return an error. This can happend, if the
	// secret which was retrieved from Vault is under a KV2 secrets engine, but
	// the secret engine was not provided in the cr for the secret. Then the
	// returned secret looks like this: &api.Secret{RequestID:\"be7b671f-a097-1081-15ec-b4710f2a6249\", LeaseID:\"\", LeaseDuration:0, Renewable:false, Data:map[string]interface {}(nil), Warnings:[]string{\"Invalid path for a versioned K/V secrets engine. See the API docs for the appropriate API endpoints to use. If using the Vault CLI, use 'vault kv get' for this operation.\"}, Auth:(*api.SecretAuth)(nil), WrapInfo:(*api.SecretWrapInfo)(nil)}"}
	if len(data) == 0 {
		return nil, ErrInvalidSecretData
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
