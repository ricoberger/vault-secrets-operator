package vault

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/hashicorp/vault/api"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	// log is our customized logger.
	log = logf.Log.WithName("vault")

	// SharedClient is our Vault client wich is used for the token auth method and the kubernetes auth method with a
	// a globally configured Vault role via the VAULT_KUBERNETES_ROLE environment variable.
	// The client is then used for all requests against Vault, except for secrets, which have the vaultRole property
	// specified.
	// If the operator is configured with the kubernetes auth method, but without a VAULT_KUBERNETES_ROLE the client can
	// be nil. When the client is nil every secret must contain the vaultRole property.
	SharedClient *Client

	// ReconciliationTime specify the time in seconds after a vault secret is reconciled.
	ReconciliationTime int
)

// InitSharedClient is used to initialize the shared client, when the VAULT_KUBERNETES_ROLE is specified.
func InitSharedClient() error {
	var err error

	// Parse the environment variable for the reconciliation time. If the time is not specify we set it to 0.
	// If the reconciliation time is 0 we skip the reconciliation of a vault secret.
	// The reconciliation time can be specified via the VAULT_RECONCILIATION_TIME environment variable.
	if ReconciliationTime, err = strconv.Atoi(os.Getenv("VAULT_RECONCILIATION_TIME")); err != nil {
		log.WithValues("ReconciliationTime", 0).Info("Reconciliation will be skipped because it is 0.")
		ReconciliationTime = 0
	} else {
		log.WithValues("ReconciliationTime", ReconciliationTime).Info("Reconciliation is enabled.")
	}

	vaultKubernetesRole := os.Getenv("VAULT_KUBERNETES_ROLE")
	SharedClient, err = CreateClient(vaultKubernetesRole)
	if err != nil {
		return err
	}

	return nil
}

// CreateClient is used by the InitSharedClient and directly for a reconciliation loop to create a new Vault client.
func CreateClient(vaultKubernetesRole string) (*Client, error) {
	vaultAddress := os.Getenv("VAULT_ADDRESS")
	vaultAuthMethod := os.Getenv("VAULT_AUTH_METHOD")
	vaultToken := os.Getenv("VAULT_TOKEN")
	vaultTokenPath := os.Getenv("VAULT_TOKEN_PATH")
	vaultTokenLeaseDuration := os.Getenv("VAULT_TOKEN_LEASE_DURATION")
	vaultTokenRenewalInterval := os.Getenv("VAULT_TOKEN_RENEWAL_INTERVAL")
	vaultTokenRenewalRetryInterval := os.Getenv("VAULT_TOKEN_RENEWAL_RETRY_INTERVAL")
	vaultKubernetesPath := os.Getenv("VAULT_KUBERNETES_PATH")
	vaultAppRolePath := os.Getenv("VAULT_APP_ROLE_PATH")
	vaultRoleID := os.Getenv("VAULT_ROLE_ID")
	vaultSecretID := os.Getenv("VAULT_SECRET_ID")
	vaultTokenMaxTTL := os.Getenv("VAULT_TOKEN_MAX_TTL")
	vaultNamespace := os.Getenv("VAULT_NAMESPACE")

	// Create new Vault configuration. This configuration is used to create the
	// API client. We set the timeout of the HTTP client to 10 seconds.
	// See: https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
	config := api.DefaultConfig()
	config.Address = vaultAddress

	apiClient, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	// Check which authentication method should be used.
	if vaultAuthMethod == "token" {
		// Check the required token and the provided lease duration for the
		// token. If the values are empty or the lease duration could not be
		// parsed we return an error.
		if vaultToken == "" {
			// If the token is not passed via environment variable we check if,
			// we can load the token from a file. Therefor a volume must be
			// mounted to the container and the path to the token must be
			// provided.
			if vaultTokenPath == "" {
				return nil, fmt.Errorf("missing vault token")
			}

			t, err := ioutil.ReadFile(vaultTokenPath)
			if err != nil {
				return nil, err
			}

			vaultToken = string(t)
		}

		if vaultTokenLeaseDuration == "" {
			return nil, fmt.Errorf("missing lease duration for vault token")
		}

		tokenLeaseDuration, err := strconv.Atoi(vaultTokenLeaseDuration)
		if err != nil {
			return nil, err
		}

		tokenRenewalInterval, err := strconv.ParseFloat(vaultTokenRenewalInterval, 64)
		if err != nil {
			tokenRenewalInterval = float64(tokenLeaseDuration) * 0.5
		}

		tokenRenewalRetryInterval, err := strconv.ParseFloat(vaultTokenRenewalRetryInterval, 64)
		if err != nil {
			tokenRenewalRetryInterval = 30.0
		}

		// Set the token, which should be used for the interaction with Vault.
		apiClient.SetToken(vaultToken)

		return &Client{
			client:                    apiClient,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			rootVaultNamespace:        vaultNamespace,
		}, nil
	}

	if vaultAuthMethod == "kubernetes" {
		// Check the required mount path and role for the Kubernetes Auth
		// Method. If one of the env variable is missing we return an error.
		if vaultKubernetesPath == "" {
			return nil, fmt.Errorf("missing Kubernetes auth path")
		}

		// For the shared client the Vault role must be specified via the VAULT_KUBERNETES_ROLE environment variable.
		// If this environment variable is missing we return nil instead of an error, because the operator will work as
		// usual, when each secret specifies the vaultRole property.
		if vaultKubernetesRole == "" {
			return nil, nil
		}

		// Read the service account token value and create a map for the
		// authentication against Vault.
		kubeToken, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
		if err != nil {
			return nil, err
		}

		data := make(map[string]interface{})
		data["jwt"] = string(kubeToken)
		data["role"] = vaultKubernetesRole

		// Authenticate against vault using the Kubernetes Auth Method and set
		// the token which the client should use for further interactions with
		// Vault. We also set the lease duration of the token for the renew
		// function.
		secret, err := apiClient.Logical().Write(vaultKubernetesPath+"/login", data)
		if err != nil {
			return nil, err
		} else if secret.Auth == nil {
			return nil, fmt.Errorf("missing authentication information")
		}

		tokenLeaseDuration := secret.Auth.LeaseDuration

		tokenRenewalInterval, err := strconv.ParseFloat(vaultTokenRenewalInterval, 64)
		if err != nil {
			tokenRenewalInterval = float64(tokenLeaseDuration) * 0.5
		}

		tokenRenewalRetryInterval, err := strconv.ParseFloat(vaultTokenRenewalRetryInterval, 64)
		if err != nil {
			tokenRenewalRetryInterval = 30.0
		}

		apiClient.SetToken(secret.Auth.ClientToken)

		return &Client{
			client:                    apiClient,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			rootVaultNamespace:        vaultNamespace,
		}, nil
	}

	if vaultAuthMethod == "approle" {
		if vaultRoleID == "" {
			return nil, fmt.Errorf("missing role id for AppRole auth method")
		}
		if vaultSecretID == "" {
			return nil, fmt.Errorf("missing secret id for AppRole auth method")
		}

		appRolePath := "auth/approle"
		if vaultAppRolePath != "" {
			appRolePath = vaultAppRolePath
		}

		data := make(map[string]interface{})
		data["role_id"] = vaultRoleID
		data["secret_id"] = vaultSecretID

		// Authenticate against vault using the AppRole Auth Method and set
		// the token which the client should use for further interactions with
		// Vault. We also set the lease duration of the token for the renew
		// function.
		secret, err := apiClient.Logical().Write(appRolePath+"/login", data)
		if err != nil {
			return nil, err
		} else if secret.Auth == nil {
			return nil, fmt.Errorf("missing authentication information")
		}

		tokenLeaseDuration := secret.Auth.LeaseDuration

		tokenRenewalInterval, err := strconv.ParseFloat(vaultTokenRenewalInterval, 64)
		if err != nil {
			tokenRenewalInterval = float64(tokenLeaseDuration) * 0.5
		}

		tokenRenewalRetryInterval, err := strconv.ParseFloat(vaultTokenRenewalRetryInterval, 64)
		if err != nil {
			tokenRenewalRetryInterval = 30.0
		}

		tokenMaxTTL, err := strconv.Atoi(vaultTokenMaxTTL)
		if err != nil {
			// Vault default max TTL is 32 days, use 16 days as the reasonable default if
			// VAULT_TOKEN_MAX_TTL not set.
			// https://learn.hashicorp.com/tutorials/vault/tokens
			tokenMaxTTL = 16 * 24 * 60 * 60
		}

		apiClient.SetToken(secret.Auth.ClientToken)

		return &Client{
			client:                    apiClient,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			tokenMaxTTL:               tokenMaxTTL,
			rootVaultNamespace:        vaultNamespace,
			requestToken: func(c *Client) error {
				secret, err := apiClient.Logical().Write(appRolePath+"/login", data)
				if err != nil {
					return err
				}
				c.client.SetToken(secret.Auth.ClientToken)
				// Update token lease duration and renewal interval
				c.tokenLeaseDuration = secret.Auth.LeaseDuration
				c.tokenRenewalInterval, err = strconv.ParseFloat(vaultTokenRenewalInterval, 64)
				if err != nil {
					c.tokenRenewalInterval = float64(c.tokenLeaseDuration) * 0.5
				}
				return nil
			},
		}, nil
	}

	return nil, fmt.Errorf("invalid authentication method")
}
