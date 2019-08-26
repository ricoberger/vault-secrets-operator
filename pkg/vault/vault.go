package vault

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/vault/api"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	// ErrMissingVaultAddress is our error if the Vault address is missing.
	ErrMissingVaultAddress = errors.New("missing vault address")
	// ErrMissingVaultToken is our error if the Vault token is missing.
	ErrMissingVaultToken = errors.New("missing vault token")
	// ErrMissingVaultTokenTTL is our error if the TTL for the Vault token is missing.
	ErrMissingVaultTokenTTL = errors.New("missing ttl for vault token")

	log    = logf.Log.WithName("vault")
	client *api.Client
)

// CreateClient creates a new Vault API client.
func CreateClient() error {
	var err error
	vaultAddress := os.Getenv("VAULT_ADDRESS")
	vaultToken := os.Getenv("VAULT_TOKEN")
	vaultTokenTTL := os.Getenv("VAULT_TOKEN_TTL")

	// Validate that the Vault address, token and token TTL is provided. If one
	// value is empty or if we could not parse the token TTL we return a error.
	if vaultAddress == "" {
		return ErrMissingVaultAddress
	}

	if vaultToken == "" {
		return ErrMissingVaultToken
	}

	if vaultTokenTTL == "" {
		return ErrMissingVaultToken
	}

	if _, err := strconv.Atoi(vaultTokenTTL); err != nil {
		return err
	}

	// Create new Vault configuration. This configuration is used to create the
	// API client. After the API client was created we set the given token, to
	// authenticat against the Vault API.
	config := &api.Config{
		Address: vaultAddress,
	}

	client, err = api.NewClient(config)
	if err != nil {
		return err
	}

	client.SetToken(vaultToken)

	return nil
}

// RenewToken renews the provided token after the half of the lease time.
// Parse the VAULT_TOKEN_TTL environment variable and renew the token after the
// half of the provided time is passed.
func RenewToken() {
	if ttl, err := strconv.Atoi(os.Getenv("VAULT_TOKEN_TTL")); err == nil {
		for {
			log.Info("Renew Vault token")

			_, err := client.Auth().Token().RenewSelf(ttl)
			if err != nil {
				log.Error(err, "Could not renew token")
			}

			time.Sleep(time.Duration(float64(ttl)*0.5) * time.Second)
		}
	} else {
		log.Error(err, "Could not parse environment variable VAULT_TOKEN_TTL to type int")
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
