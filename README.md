<div align="center">
  <img src="./assets/logo.png" width="20%" />
  <br><br>

  Create Kubernetes secrets from Vault.

  <img src="./assets/gitops.png" width="100%" />
</div>

The **Vault Secrets Operator** creates a Kubernetes secret from a Vault. The idea behind the Vault Secrets Operator is to manage secrets in Kubernetes using a secure GitOps based workflow. The Vault Secrets Operator reads a Vault secret from the defined path in a CR and creates a Kubernetes secret from it. The Operator supports the **Token Auth Method** and the **KV Secrets Engine - Version 1** from Vault.

## Installation

First of all you have to create a new **KV Secrets Engine - Version 1**. To create the Secret Engine under a path named `secrets`, you can run the following command:

```sh
vault secrets enable -path=secrets -version=1 kv
```

After you have enabled the Secrets Engine, create a new policy for the Vault Secrets Operator. The Operator only needs read access to the paths you want to use for your secrets. To create a new policy with the name `vault-secrets-operator` and read access to the `secrets` path, you can run the following command:

```sh
cat <<EOF | vault policy write vault-secrets-operator -
path "secrets/*" {
  capabilities = ["read"]
}
EOF
```

The Vault Secrets Operator uses **Token Auth Method** for the authentication against the Vault API. To create a Token for the Operator you can run the following command:

```sh
vault token create -period=168h -policy=vault-secrets-operator
```

The Operator requires three environment variables: `VAULT_ADDRESS`, `VAULT_TOKEN` and `VAULT_TOKEN_TTL`. These should be set by a Kubernetes secret. The Helm chart for the Operator requires a secret named `vault-secrets-operator` with the environment variables as keys. You can use the following commands to create this secret:

```sh
export VAULT_ADDRESS=
export VAULT_TOKEN=
export VAULT_TOKEN_TTL=604800

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: vault-secrets-operator
type: Opaque
data:
  VAULT_ADDRESS: $(echo -n "$VAULT_ADDRESS" | base64)
  VAULT_TOKEN: $(echo -n "$VAULT_TOKEN" | base64)
  VAULT_TOKEN_TTL: $(echo -n "$VAULT_TOKEN_TTL" | base64)
EOF
```

When the secret was created, you can deploy the Helm chart for the Vault Secrets Operator using the following command:

```sh
helm repo add ricoberger https://ricoberger.github.io/helm-charts
helm repo update

helm upgrade --install vault-secrets-operator ricoberger/vault-secrets-operator
```

## Usage

The following usage examples will use the following Vault secret:

```sh
vault kv put secrets/example-vaultsecret foo=bar hello=world
```

To deploy the `example-vaultsecret` to your Kubernetes cluster, you can use the following CR:

```yaml
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: example-vaultsecret
spec:
  keys:
    - foo
  path: secrets/example-vaultsecret
  type: Opaque
```

The Vault Secrets Operator creates a Kubernetes secret named `example-vaultsecret` with the type `Opaque` from this CR:

```yaml
apiVersion: v1
data:
  foo: YmFy
kind: Secret
metadata:
  labels:
    created-by: vault-secrets-operator
  name: example-vaultsecret
type: Opaque
```

You can also omit the `keys` spec to create a Kubernetes secret which contains all keys from the Vault secret:

```yaml
apiVersion: v1
data:
  foo: YmFy
  hello: d29ybGQ=
kind: Secret
metadata:
  labels:
    created-by: vault-secrets-operator
  name: example-vaultsecret
type: Opaque
```

## Development

After modifying the `*_types.go` file always run the following command to update the generated code for that resource type:

```sh
operator-sdk generate k8s
```

To update the OpenAPI validation section in the CRD `deploy/crds/cache_v1alpha1_memcached_crd.yaml`, run the following command.

```sh
operator-sdk generate openapi
```

Create an example secret in Vault. Then apply the Custom Resource Definition for the Vault Secrets Operator and the example Custom Resource:

```sh
vault kv put secrets/example-vaultsecret foo=bar

kubectl apply -f deploy/crds/ricoberger_v1alpha1_vaultsecret_crd.yaml
kubectl apply -f deploy/crds/ricoberger_v1alpha1_vaultsecret_cr.yaml
```

Set the name of the operator in an environment variable:

```sh
export OPERATOR_NAME=vault-secrets-operator
```

Specify the Vault address, a token to access Vault and the TTL (in seconds) for the token:

```sh
export VAULT_ADDRESS=
export VAULT_TOKEN=
export VAULT_TOKEN_TTL=
```

Run the operator locally with the default Kubernetes config file present at `$HOME/.kube/config`:

```sh
operator-sdk up local --namespace=default
```

You can use a specific kubeconfig via the flag `--kubeconfig=<path/to/kubeconfig>`.

## Links

- [Managing Secrets in Kubernetes](https://www.weave.works/blog/managing-secrets-in-kubernetes)
- [Operator SDK](https://github.com/operator-framework/operator-sdk)
- [Vault](https://www.vaultproject.io)
