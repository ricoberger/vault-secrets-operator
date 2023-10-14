package vault

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	gcpmetadata "cloud.google.com/go/compute/metadata"
	gcpcredentials "cloud.google.com/go/iam/credentials/apiv1"
	"github.com/aws/aws-sdk-go/aws"
	awscredentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	awsdefaults "github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/vault/api"
	"github.com/leosayous21/go-azure-msi/msi"
	"github.com/pkg/errors"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iam/v1"
	gcpcredentialspb "google.golang.org/genproto/googleapis/iam/credentials/v1"
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
	vaultHeader := os.Getenv("VAULT_HEADER")
	vaultAuthMethod := os.Getenv("VAULT_AUTH_METHOD")
	vaultUser := os.Getenv("VAULT_USER")
	vaultPassword := os.Getenv("VAULT_PASSWORD")
	vaultToken := os.Getenv("VAULT_TOKEN")
	vaultTokenPath := os.Getenv("VAULT_TOKEN_PATH")
	vaultTokenLeaseDuration := os.Getenv("VAULT_TOKEN_LEASE_DURATION")
	vaultRenewToken := os.Getenv("VAULT_RENEW_TOKEN")
	vaultTokenRenewalInterval := os.Getenv("VAULT_TOKEN_RENEWAL_INTERVAL")
	vaultTokenRenewalRetryInterval := os.Getenv("VAULT_TOKEN_RENEWAL_RETRY_INTERVAL")
	vaultKubernetesPath := os.Getenv("VAULT_KUBERNETES_PATH")
	vaultAppRolePath := os.Getenv("VAULT_APP_ROLE_PATH")
	vaultAzurePath := os.Getenv("VAULT_AZURE_PATH")
	vaultAzureRole := os.Getenv("VAULT_AZURE_ROLE")
	vaultAzureIsScaleset := os.Getenv("VAULT_AZURE_ISSCALESET")
	vaultAwsRegion := os.Getenv("VAULT_AWS_REGION")
	vaultAwsPath := os.Getenv("VAULT_AWS_PATH")
	vaultAwsAuthType := os.Getenv("VAULT_AWS_AUTH_TYPE")
	vaultAwsRole := os.Getenv("VAULT_AWS_ROLE")
	vaultGcpPath := os.Getenv("VAULT_GCP_PATH")
	vaultGcpAuthType := os.Getenv("VAULT_GCP_AUTH_TYPE")
	vaultGcpRole := os.Getenv("VAULT_GCP_ROLE")
	vaultGcpServiceAccountEmail := os.Getenv("VAULT_GCP_SERVICE_ACCOUNT_EMAIL")
	vaultTokenMaxTTL := os.Getenv("VAULT_TOKEN_MAX_TTL")
	vaultNamespace := os.Getenv("VAULT_NAMESPACE")
	vaultPKIRenew := os.Getenv("VAULT_PKI_RENEW")

	// Create new Vault configuration. This configuration is used to create the
	// API client. We set the timeout of the HTTP client to 10 seconds.
	// See: https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
	config := api.DefaultConfig()
	config.Address = vaultAddress

	apiClient, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	renewToken, err := strconv.ParseBool(vaultRenewToken)
	if err != nil {
		renewToken = true
	}

	if len(vaultPKIRenew) == 0 {
		vaultPKIRenew = "1h"
	}

	pkiRenew, err := time.ParseDuration(vaultPKIRenew)
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
			renewToken:                renewToken,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			rootVaultNamespace:        vaultNamespace,
			pkiRenew:                  pkiRenew,
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
			renewToken:                renewToken,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			rootVaultNamespace:        vaultNamespace,
			pkiRenew:                  pkiRenew,
		}, nil
	}

	if vaultAuthMethod == "approle" {
		vaultRoleID := setVaultIDs("role")
		vaultSecretID := setVaultIDs("secret")

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
			renewToken:                renewToken,
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
			pkiRenew: pkiRenew,
		}, nil
	}

	if vaultAuthMethod == "userpass" {
		if vaultUser == "" {
			return nil, fmt.Errorf("missing username for userpass auth method")
		}
		if vaultPassword == "" {
			return nil, fmt.Errorf("missing password for userpass auth method")
		}

		data := make(map[string]interface{})
		data["password"] = vaultPassword

		userPassLoginPath := "auth/userpass/login/" + vaultUser
		secret, err := apiClient.Logical().Write(userPassLoginPath, data)
		if err != nil {
			return nil, err
		}
		if secret.Auth == nil {
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
			renewToken:                renewToken,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			tokenMaxTTL:               tokenMaxTTL,
			rootVaultNamespace:        vaultNamespace,
			requestToken: func(c *Client) error {
				secret, err := apiClient.Logical().Write(userPassLoginPath, data)
				if err != nil {
					return err
				}
				if secret.Auth == nil {
					return fmt.Errorf("missing authentication information")
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
			pkiRenew: pkiRenew,
		}, nil
	}

	if vaultAuthMethod == "azure" {
		// Check the required mount path and role for the Kubernetes Auth
		// Method. If one of the env variable is missing we return an error.
		if vaultAzurePath == "" {
			vaultAzurePath = "auth/azure"
		}

		// For the shared client the Vault role must be specified via the VAULT_KUBERNETES_ROLE environment variable.
		// If this environment variable is missing we return nil instead of an error, because the operator will work as
		// usual, when each secret specifies the vaultRole property.
		if vaultAzureRole == "" {
			vaultAzureRole = "default"
		}

		// Read the service account token value and create a map for the
		// authentication against Vault.
		msiToken, err := msi.GetMsiToken()
		if err != nil {
			return nil, err
		}
		metadata, err := msi.GetInstanceMetadata()
		if err != nil {
			return nil, err
		}

		data := make(map[string]interface{})
		data["jwt"] = string(msiToken.AccessToken)
		data["role"] = vaultAzureRole
		data["subscription_id"] = metadata.SubscriptionId
		data["resource_group_name"] = metadata.ResourceGroupName
		if vaultAzureIsScaleset == "true" {
			data["vmss_name"] = metadata.VMssName
		} else {
			data["vm_name"] = metadata.VMName
		}

		// Authenticate against vault using the Azure Auth Method and set
		// the token which the client should use for further interactions with
		// Vault. We also set the lease duration of the token for the renew
		// function.
		secret, err := apiClient.Logical().Write(vaultAzurePath+"/login", data)
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
			renewToken:                renewToken,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			rootVaultNamespace:        vaultNamespace,
			pkiRenew:                  pkiRenew,
		}, nil

	}

	if vaultAuthMethod == "aws" {
		// Check the required mount path and role for the AWS Auth
		// Method. If one of the env variable is missing we return an error.
		if vaultAwsPath == "" {
			vaultAwsPath = "auth/aws"
		}

		var awsLoginDataFunc func() (map[string]interface{}, error)

		switch vaultAwsAuthType {
		case "ec2":
			awsLoginDataFunc = func() (map[string]interface{}, error) {
				sess, err := awssession.NewSession()
				if err != nil {
					return nil, errors.Wrap(err, "error creating a new session to create ec2metadata")
				}
				metadataSvc := ec2metadata.New(sess)
				doc, err := metadataSvc.GetDynamicData("/instance-identity/document")
				if err != nil {
					return nil, fmt.Errorf("error requesting doc: %w", err)
				}

				signature, err := metadataSvc.GetDynamicData("/instance-identity/signature")
				if err != nil {
					return nil, fmt.Errorf("error requesting signature: %w", err)
				}

				kubeToken, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
				if err != nil {
					return nil, err
				}
				if err != nil {
					return nil, err
				}

				nonce := fmt.Sprintf("%x", sha256.Sum256(kubeToken))

				return map[string]interface{}{
					"identity":  base64.StdEncoding.EncodeToString([]byte(doc)),
					"signature": signature,
					"nonce":     nonce,
					"role":      vaultAwsRole,
				}, nil
			}
		case "iam":
			awsLoginDataFunc = func() (map[string]interface{}, error) {
				var providers []awscredentials.Provider

				// Load in AWS env variables if exist.
				roleARN := os.Getenv("AWS_ROLE_ARN")
				tokenPath := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")
				roleSessionName := os.Getenv("AWS_ROLE_SESSION_NAME")

				if roleARN != "" && tokenPath != "" {
					sess, err := awssession.NewSession()
					if err != nil {
						return nil, errors.Wrap(err, "error creating a new session to create a WebIdentityRoleProvider")
					}
					webIdentityProvider := stscreds.NewWebIdentityRoleProvider(sts.New(sess), roleARN, roleSessionName, tokenPath)

					// Add the web identity role credential provider
					providers = append(providers, webIdentityProvider)
				}

				// Add the environment credential provider
				providers = append(providers, &awscredentials.EnvProvider{})

				// Add the remote provider
				def := awsdefaults.Get()
				providers = append(providers, awsdefaults.RemoteCredProvider(*def.Config, def.Handlers))

				// Create the credentials required to access the API.
				creds := awscredentials.NewChainCredentials(providers)
				if creds == nil {
					return nil, fmt.Errorf("could not compile valid credential providers from environment, web identity or instance metadata")
				}

				stsSession, err := awssession.NewSessionWithOptions(awssession.Options{
					Config: aws.Config{
						Credentials:      creds,
						Region:           aws.String(vaultAwsRegion),
						EndpointResolver: endpoints.ResolverFunc(stsSigningResolver),
					},
				})
				if err != nil {
					return nil, err
				}

				var params *sts.GetCallerIdentityInput
				svc := sts.New(stsSession)
				stsRequest, _ := svc.GetCallerIdentityRequest(params)
				stsRequest.HTTPRequest.Header.Add("X-Vault-AWS-IAM-Server-ID", vaultHeader)

				// Sign the request
				stsRequest.Sign()

				// Now extract out the relevant parts of the request
				headersJson, err := json.Marshal(stsRequest.HTTPRequest.Header)
				if err != nil {
					return nil, err
				}
				requestBody, err := ioutil.ReadAll(stsRequest.HTTPRequest.Body)
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"iam_http_request_method": stsRequest.HTTPRequest.Method,
					"iam_request_url":         base64.StdEncoding.EncodeToString([]byte(stsRequest.HTTPRequest.URL.String())),
					"iam_request_headers":     base64.StdEncoding.EncodeToString(headersJson),
					"iam_request_body":        base64.StdEncoding.EncodeToString(requestBody),
					"role":                    vaultAwsRole,
				}, nil
			}
		default:
			awsLoginDataFunc = func() (map[string]interface{}, error) {
				return nil, fmt.Errorf("invalid aws authentication type")
			}
		}

		// Create login data according to AWS Auth Type
		data, err := awsLoginDataFunc()
		if err != nil {
			return nil, err
		}

		// Authenticate against vault using the AWS Auth Method and set
		// the token which the client should use for further interactions with
		// Vault. We also set the lease duration of the token for the renew
		// function.
		secret, err := apiClient.Logical().Write(vaultAwsPath+"/login", data)
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

		// Tokens have to be reissued after a short period
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
			renewToken:                renewToken,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			rootVaultNamespace:        vaultNamespace,
			tokenMaxTTL:               tokenMaxTTL,
			requestToken: func(c *Client) error {
				data, err := awsLoginDataFunc()
				if err != nil {
					return err
				}
				secret, err := apiClient.Logical().Write(vaultAwsPath+"/login", data)
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
			pkiRenew: pkiRenew,
		}, nil
	}

	if vaultAuthMethod == "gcp" {

		// Check the required mount path and role for the GCP Auth
		// Method. If one of the env variable is missing we return an error.
		if vaultGcpPath == "" {
			vaultGcpPath = "auth/gcp"
		}

		var gcpLoginDataFunc func() (map[string]interface{}, error)

		switch vaultGcpAuthType {
		case "gce":
			gcpLoginDataFunc = func() (map[string]interface{}, error) {
				// Read the service account token value and create a map for the
				// authentication against Vault.
				tokenSource, err := google.DefaultTokenSource(context.TODO(), iam.CloudPlatformScope)
				if err != nil {
					return nil, err
				}
				jwt, err := tokenSource.Token()
				if err != nil {
					return nil, err
				}

				return map[string]interface{}{
					"jwt":  jwt.AccessToken,
					"role": vaultGcpRole,
				}, nil
			}
		case "iam":
			gcpLoginDataFunc = func() (map[string]interface{}, error) {
				// Read the service account token value and create a map for the
				// authentication against Vault.
				c, err := gcpcredentials.NewIamCredentialsClient(context.TODO())
				if err != nil {
					return nil, fmt.Errorf("could not create IAM client: %w", err)
				}

				if vaultGcpServiceAccountEmail == "" {
					metadataClient := gcpmetadata.NewClient(nil)
					vaultGcpServiceAccountEmail, err = metadataClient.Email("default")
					if err != nil {
						return nil, fmt.Errorf("could not obtain service account from credentials; a service account to authenticate as must be provided")
					}
				}

				ttl := time.Minute * time.Duration(15)
				jwtPayload := map[string]interface{}{
					"aud": fmt.Sprintf("vault/%s", vaultGcpRole),
					"sub": vaultGcpServiceAccountEmail,
					"exp": time.Now().Add(ttl).Unix(),
				}

				payloadBytes, err := json.Marshal(jwtPayload)
				if err != nil {
					return nil, fmt.Errorf("could not convert JWT payload to JSON string: %w", err)
				}

				resourceName := fmt.Sprintf("projects/-/serviceAccounts/%s", vaultGcpServiceAccountEmail)
				req := &gcpcredentialspb.SignJwtRequest{
					Name:    resourceName,
					Payload: string(payloadBytes),
				}
				resp, err := c.SignJwt(context.TODO(), req)
				if err != nil {
					return nil, fmt.Errorf("unable to sign JWT for %s using given Vault credentials: %w", resourceName, err)
				}

				return map[string]interface{}{
					"jwt":  resp.SignedJwt,
					"role": vaultGcpRole,
				}, nil
			}
		default:
			gcpLoginDataFunc = func() (map[string]interface{}, error) {
				return nil, fmt.Errorf("invalid gcp authentication type")
			}
		}

		// Create login data according to GCP Auth Type
		data, err := gcpLoginDataFunc()
		if err != nil {
			return nil, err
		}

		// Authenticate against vault using the GCP Auth Method and set
		// the token which the client should use for further interactions with
		// Vault. We also set the lease duration of the token for the renew
		// function.
		secret, err := apiClient.Logical().Write(vaultGcpPath+"/login", data)
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
			renewToken:                renewToken,
			tokenLeaseDuration:        tokenLeaseDuration,
			tokenRenewalInterval:      tokenRenewalInterval,
			tokenRenewalRetryInterval: tokenRenewalRetryInterval,
			rootVaultNamespace:        vaultNamespace,
			requestToken: func(c *Client) error {
				data, err := gcpLoginDataFunc()
				if err != nil {
					return err
				}
				secret, err := apiClient.Logical().Write(vaultGcpPath+"/login", data)
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
			pkiRenew: pkiRenew,
		}, nil
	}

	return nil, fmt.Errorf("invalid authentication method")
}

func stsSigningResolver(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
	defaultEndpoint, err := endpoints.DefaultResolver().EndpointFor(service, region, optFns...)
	if err != nil {
		return defaultEndpoint, err
	}
	defaultEndpoint.SigningRegion = region
	return defaultEndpoint, nil
}

func setVaultIDs(idType string) string {
	var idPath string

	if idType == "role" {
		id, found := os.LookupEnv("VAULT_ROLE_ID")
		if found {
			return id
		}
		idPath = os.Getenv("VAULT_ROLE_ID_PATH")
	}

	if idType == "secret" {
		id, found := os.LookupEnv("VAULT_SECRET_ID")
		if found {
			return id
		}
		idPath = os.Getenv("VAULT_SECRET_ID_PATH")
	}

	id, err := ioutil.ReadFile(idPath)
	if err != nil {
		log.WithValues("VaultFilePath", idPath).Error(err, "missing secret vault-secrets-operator or bad path in volume")
		return string(id)
	}

	return string(id)
}
