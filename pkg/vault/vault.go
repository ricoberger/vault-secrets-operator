package vault

import (
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/vault/api"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log    = logf.Log.WithName("vault")
	client *api.Client
)

// createClient creates a new Vault API client.
func createClient() {
	config := &api.Config{
		Address: os.Getenv("VAULT_ADDRESS"),
	}

	c, err := api.NewClient(config)
	if err != nil {
		log.Error(err, "Could not create API client")
	}

	c.SetToken(os.Getenv("VAULT_TOKEN"))

	log.Info("Vault API client was created")

	client = c
}

// GetSecret returns the value for a given secret.
func GetSecret(path string) (map[string][]byte, error) {
	// If the client does not exist we create one.
	if client == nil {
		createClient()
	}

	// Get the secret for the given path and return the secret data.
	log.Info(fmt.Sprintf("Read secret %s", path))

	secret, err := client.Logical().Read(path)
	if err != nil {
		return nil, err
	}

	if secret == nil {
		return nil, errors.New("could not get secret")
	}

	// Convert the secret data for a Kubernetes secret.
	data := make(map[string][]byte)
	for key, value := range secret.Data {
		if valueStr, ok := value.(string); ok {
			data[key] = []byte(valueStr)
		}
	}

	return data, nil
}
