package vault

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/hashicorp/vault/api"
	authaws "github.com/hashicorp/vault/api/auth/aws"
)

// awsAuthConfig holds the configuration for the Vault AWS auth method.
type awsAuthConfig struct {
	// authType is the AWS authentication type, either "iam" or "ec2".
	authType string
	// region is the AWS region which is used to sign the sts:GetCallerIdentity
	// request for the "iam" auth type.
	region string
	// role is the name of the Vault role to authenticate against.
	role string
	// mountPath is the Vault auth mount path without the "auth/" prefix, e.g.
	// "aws".
	mountPath string
	// serverIDHeader is the value for the X-Vault-AWS-IAM-Server-ID header which
	// is added to the signed sts:GetCallerIdentity request ("iam" auth type
	// only). It can be empty.
	serverIDHeader string
	// kubeTokenPath is the path to the Kubernetes service account token, which
	// is used to derive the reauthentication nonce for the "ec2" auth type.
	kubeTokenPath string
}

// awsLogin authenticates against the Vault AWS auth method using the official
// hashicorp/vault/api/auth/aws library. The library builds and signs the
// sts:GetCallerIdentity request for the "iam" auth type, or reads the instance
// identity document from the EC2 Instance Metadata Service for the "ec2" auth
// type, and writes the login data to Vault.
func awsLogin(ctx context.Context, client *api.Client, cfg awsAuthConfig) (*api.Secret, error) {
	opts := []authaws.LoginOption{
		authaws.WithMountPath(cfg.mountPath),
		authaws.WithRegion(cfg.region),
	}
	if cfg.role != "" {
		opts = append(opts, authaws.WithRole(cfg.role))
	}
	if cfg.serverIDHeader != "" {
		opts = append(opts, authaws.WithIAMServerIDHeader(cfg.serverIDHeader))
	}

	switch cfg.authType {
	case "ec2":
		// Derive a deterministic reauthentication nonce from the Kubernetes
		// service account token, matching the behaviour of previous versions of
		// the operator. WithIdentitySignature keeps using the instance identity
		// document and its signature (instead of the default PKCS #7 signature).
		nonce, err := awsEC2Nonce(cfg.kubeTokenPath)
		if err != nil {
			return nil, err
		}
		opts = append(opts,
			authaws.WithEC2Auth(),
			authaws.WithIdentitySignature(),
			authaws.WithNonce(nonce),
		)
	case "iam":
		// The library reads the AWS credentials from the AWS_ACCESS_KEY_ID,
		// AWS_SECRET_ACCESS_KEY and AWS_SESSION_TOKEN environment variables. We
		// resolve them via the AWS SDK v2 default credential chain (which
		// supports IRSA / web identity tokens, EKS Pod Identity, environment
		// variables and shared config) and export them so that they are picked
		// up by the library.
		if err := exportAWSCredentials(ctx, cfg.region); err != nil {
			return nil, err
		}
		opts = append(opts, authaws.WithIAMAuth())
	default:
		return nil, fmt.Errorf("invalid aws authentication type")
	}

	awsAuth, err := authaws.NewAWSAuth(opts...)
	if err != nil {
		return nil, err
	}

	secret, err := awsAuth.Login(ctx, client)
	if err != nil {
		return nil, err
	}
	if secret == nil || secret.Auth == nil {
		return nil, fmt.Errorf("missing authentication information")
	}

	return secret, nil
}

// awsEC2Nonce derives a deterministic reauthentication nonce from the
// Kubernetes service account token.
func awsEC2Nonce(kubeTokenPath string) (string, error) {
	kubeToken, err := os.ReadFile(kubeTokenPath)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", sha256.Sum256(kubeToken)), nil
}

// exportAWSCredentials resolves AWS credentials using the AWS SDK v2 default
// credential chain and exports them to the environment so that the
// hashicorp/vault/api/auth/aws library can use them to sign the
// sts:GetCallerIdentity request.
func exportAWSCredentials(ctx context.Context, region string) error {
	var optFns []func(*awsconfig.LoadOptions) error
	if region != "" {
		optFns = append(optFns, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return err
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return err
	}

	if err := os.Setenv("AWS_ACCESS_KEY_ID", creds.AccessKeyID); err != nil {
		return err
	}
	if err := os.Setenv("AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey); err != nil {
		return err
	}
	if err := os.Setenv("AWS_SESSION_TOKEN", creds.SessionToken); err != nil {
		return err
	}

	return nil
}
