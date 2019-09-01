package vault

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
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
func GetSecret(path string, keys []string) (map[string][]byte, error) {
	// Get the secret for the given path and return the secret data.
	log.Info(fmt.Sprintf("Read secret %s", path))

	secret, err := client.Logical().Read(path)
	if err != nil {
		return nil, err
	}

	if secret == nil {
		return nil, errors.New("could not get secret")
	}

	// Convert the secret data for a Kubernetes secret. We only add the provided
	// keys to the resulting data or if there are no keys provided we add all
	// keys of the secret.
	data := make(map[string][]byte)
	for key, value := range secret.Data {
		if len(keys) == 0 || contains(key, keys) {
			if valueStr, ok := value.(string); ok {
				data[key] = []byte(valueStr)
			}
		}
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
