<div align="center">
  <img src="./assets/logo.png" width="20%" />
  <br><br>

  Create Kubernetes secrets from Vault for a secure GitOps based workflow.

  <img src="./assets/gitops.png" width="100%" />
</div>

The **Vault Secrets Operator** creates Kubernetes secrets from Vault. The idea behind the Vault Secrets Operator is to manage secrets in Kubernetes cluster using a secure GitOps based workflow. For more information about a secure GitOps based workflow I recommend the article ["Managing Secrets in Kubernetes"](https://www.weave.works/blog/managing-secrets-in-kubernetes) from [Weaveworks](https://www.weave.works). With the help of the Vault Secrets Operator you can commit your secrets to your git repository using a custom resource. If you apply these secrets to your Kubernetes cluster the Operator will lookup the real secret in Vault and creates the corresponding Kubernetes secret. If you are using something like [Sealed Secrets](http://github.com/bitnami-labs/sealed-secrets) for this workflow the Vault Secrets Operator can be used as replacement for this.

## Installation

The Vault Secrets Operator can be installed via Helm. A list of all configurable values can be found [here](./charts/README.md). The chart assumes a vault server running at `http://vault:8200`, but can be overidden by specifying `--set vault.address=https://vault.example.com`

```sh
helm repo add ricoberger https://ricoberger.github.io/helm-charts
helm repo update

helm upgrade --install vault-secrets-operator ricoberger/vault-secrets-operator
```

### Prepare Vault

The Vault Secrets Operator supports the **KV Secrets Engine - Version 1** and **KV Secrets Engine - Version 2**. To create a new secret engine under a path named `kvv1` and `kvv2`, you can run the following command:

```sh
vault secrets enable -path=kvv1 -version=1 kv
vault secrets enable -path=kvv2 -version=2 kv
```

After you have enabled the secret engine, create a new policy for the Vault Secrets Operator. The operator only needs read access to the paths you want to use for your secrets. To create a new policy with the name `vault-secrets-operator` and read access to the `kvv1` and `kvv2` path, you can run the following command:

```sh
cat <<EOF | vault policy write vault-secrets-operator -
path "kvv1/*" {
  capabilities = ["read"]
}

path "kvv2/data/*" {
  capabilities = ["read"]
}
EOF
```

To access Vault the operator can choose between the **[Token Auth Method](https://www.vaultproject.io/docs/auth/token.html)** or the **[Kubernetes Auth Method](https://www.vaultproject.io/docs/auth/kubernetes.html)**. In the next sections you found the instructions to setup Vault for the two authentication methods.

#### Token Auth Method

To use Token auth method for the authentication against the Vault API, you need to create a token. A token with the previously created policy can be created as follows:

```sh
vault token create -period=24h -policy=vault-secrets-operator
```

To use the created token you need to pass the token as environment variable to the operator. For security reaseons the operator only supports the passing of environment variables via a Kubernetes secret. The secret with the keys `VAULT_TOKEN` and `VAULT_TOKEN_LEASE_DURATION` can be created with the following command:

```sh
export VAULT_TOKEN=
export VAULT_TOKEN_LEASE_DURATION=86400

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: vault-secrets-operator
type: Opaque
data:
  VAULT_TOKEN: $(echo -n "$VAULT_TOKEN" | base64)
  VAULT_TOKEN_LEASE_DURATION: $(echo -n "$VAULT_TOKEN_LEASE_DURATION" | base64)
EOF
```

This creates a secret named `vault-secrets-operator`. To use this secret in the Helm chart modify the `values.yaml` file as follows:

```yaml
environmentVars:
  - name: VAULT_TOKEN
    valueFrom:
      secretKeyRef:
        name: vault-secrets-operator
        key: VAULT_TOKEN
  - name: VAULT_TOKEN_LEASE_DURATION
    valueFrom:
      secretKeyRef:
        name: vault-secrets-operator
        key: VAULT_TOKEN_LEASE_DURATION
```

#### Kubernetes Auth Method

The recommanded way for the authentication is the Kubernetes auth method. There for you need a service account for the communication between Vault and the Vault Secrets Operator. If you installed the operator via Helm this service account is created for you. The name of the created service account is `vault-secrets-operator`. Use the following commands to set the environment variables for the activation of the Kubernetes auth method:

```sh
export VAULT_SECRETS_OPERATOR_NAMESPACE=$(kubectl get sa vault-secrets-operator -o jsonpath="{.metadata.namespace}")
export VAULT_SECRET_NAME=$(kubectl get sa vault-secrets-operator -o jsonpath="{.secrets[*]['name']}")
export SA_JWT_TOKEN=$(kubectl get secret $VAULT_SECRET_NAME -o jsonpath="{.data.token}" | base64 --decode; echo)
export SA_CA_CRT=$(kubectl get secret $VAULT_SECRET_NAME -o jsonpath="{.data['ca\.crt']}" | base64 --decode; echo)
export K8S_HOST=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')

# Verfify the environment variables
env | grep -E 'VAULT_SECRETS_OPERATOR_NAMESPACE|VAULT_SECRET_NAME|SA_JWT_TOKEN|SA_CA_CRT|K8S_HOST'
```

Enable the Kubernetes auth method at the default path (`auth/kubernetes`) and finish the configuration of Vault:

```sh
vault auth enable kubernetes

# Tell Vault how to communicate with the Kubernetes cluster
vault write auth/kubernetes/config \
  token_reviewer_jwt="$SA_JWT_TOKEN" \
  kubernetes_host="$K8S_HOST" \
  kubernetes_ca_cert="$SA_CA_CRT"

# Create a role named, 'vault-secrets-operator' to map Kubernetes Service Account to Vault policies and default token TTL
vault write auth/kubernetes/role/vault-secrets-operator \
  bound_service_account_names="vault-secrets-operator" \
  bound_service_account_namespaces="$VAULT_SECRETS_OPERATOR_NAMESPACE" \
  policies=vault-secrets-operator \
  ttl=24h
```

## Usage

Create two Vault secrets `example-vaultsecret`:

```sh
vault kv put kvv1/example-vaultsecret foo=bar hello=world

vault kv put kvv2/example-vaultsecret foo=bar
vault kv put kvv2/example-vaultsecret hello=world
vault kv put kvv2/example-vaultsecret foo=bar hello=world
```

Deploy the custom resource `kvv1-example-vaultsecret` to your Kubernetes cluster:

```yaml
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: kvv1-example-vaultsecret
spec:
  keys:
    - foo
  path: kvv1/example-vaultsecret
  type: Opaque
```

The Vault Secrets Operator creates a Kubernetes secret named `kvv1-example-vaultsecret` with the type `Opaque` from this CR:

```yaml
apiVersion: v1
data:
  foo: YmFy
kind: Secret
metadata:
  labels:
    created-by: vault-secrets-operator
  name: kvv1-example-vaultsecret
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
  name: kvv1-example-vaultsecret
type: Opaque
```

To deploy a custom resource `kvv2-example-vaultsecret`, which uses the secret from the KV Secrets Engine - Version 2 you can use the following:

```yaml
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: kvv2-example-vaultsecret
spec:
  path: kvv2/example-vaultsecret
  type: Opaque
```

The Vault Secrets Operator will create a secret which looks like the following:

```yaml
apiVersion: v1
data:
  foo: YmFy
  hello: d29ybGQ=
kind: Secret
metadata:
  labels:
    created-by: vault-secrets-operator
  name: kvv2-example-vaultsecret
type: Opaque
```

For secrets using the KVv2 secret engine you can also specify the version of the secret you want to deploy:

```yaml
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: kvv2-example-vaultsecret
spec:
  path: kvv2/example-vaultsecret
  type: Opaque
  version: 2
```

The resulting Kubernetes secret will be:

```yaml
apiVersion: v1
data:
  hello: d29ybGQ=
kind: Secret
metadata:
  labels:
    created-by: vault-secrets-operator
  name: kvv2-example-vaultsecret
type: Opaque
```

The `spec.type` and `spec.keys` fields are handled in the same way for both versions of the KV secret engine. The `spec.version` field is only processed, when the secret is saved under a KVv2 secret engine. If you specified the `VAULT_RECONCILIATION_TIME` environment variable with a value greater than `0` every secret is reconciled after the given time. This means, when you do not specify `spec.version`, the Kubernetes secret will be automatically updated if the Vault secret changes.

The binary data stored in vault requires [base64 encoding](https://github.com/hashicorp/vault/issues/1423#issuecomment-219525845). the
`spec.isBinary` can be used to prevent such data get base64 encoded again when store as secret in k8s.

For example, let's set `foo` to the `bar` in base64 encoded format (i.e. YmFyCg==).

```sh
vault kv put kvv1/example-vaultsecret foo=YmFyCg==
```

You can specify `spec.isBinary` to indicate this is a binary data which is already in base64 encoded format:

```yaml
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: kvv1-example-vaultsecret
spec:
  keys:
    - foo
  isBinary: true
  path: kvv1/example-vaultsecret
  type: Opaque
```

The resulting Kubernetes secret will be:

```yaml
apiVersion: v1
data:
  foo: YmFyCg==
kind: Secret
metadata:
  labels:
    created-by: vault-secrets-operator
  name: kvv1-example-vaultsecret
type: Opaque
```

The value for `foo` stays as `YmFyCg==` which does not get base64 encoded again.

It is also possible to change the default reconciliation strategy from `Replace` to `Merge` via the `reconcileStrategy` key in the CRD. For the default `Replace` strategy the complete secret is replaced. If you have an existing secret you can choose the `Merge` strategy to add the keys from Vault to the existing secret.

## Development

After modifying the `*_types.go` file always run the following command to update the generated code for that resource type:

```sh
operator-sdk generate k8s
```

To update the CRD `deploy/crds/ricoberger.de_vaultsecrets_crd.yaml`, run the following command:

```sh
operator-sdk generate crds
```

Create an example secret in Vault. Then apply the Custom Resource Definition for the Vault Secrets Operator and the example Custom Resource:

```sh
vault kv put kvv1/example-vaultsecret foo=bar

kubectl apply -f deploy/crds/ricoberger.de_vaultsecrets_crd.yaml
kubectl apply -f deploy/crds/ricoberger.de_v1alpha1_vaultsecret_cr.yaml
```

### Locally

Set the name of the operator in an environment variable:

```sh
export OPERATOR_NAME=vault-secrets-operator
```

Specify the Vault address, a token to access Vault and the TTL (in seconds) for the token:

```sh
export VAULT_ADDRESS=
export VAULT_AUTH_METHOD=token
export VAULT_TOKEN=
export VAULT_TOKEN_LEASE_DURATION=
export VAULT_RECONCILIATION_TIME=
```

Run the operator locally with the default Kubernetes config file present at `$HOME/.kube/config`:

```sh
operator-sdk run local --watch-namespace=""
```

You can use a specific kubeconfig via the flag `--kubeconfig=<path/to/kubeconfig>`.

### Minikube

Reuse Minikube’s built-in Docker daemon:

```sh
eval $(minikube docker-env)
```

Build the Docker image for the operator:

```sh
make build
```

Deploy the Helm chart:

```sh
cat <<EOF | helm upgrade --install vault-secrets-operator ./charts/vault-secrets-operator -f -
image:
  repository: ricoberger/vault-secrets-operator
  tag:
  args: ["--zap-encoder", "console"]

vault:
  address: ""
  authMethod: "kubernetes"
EOF
```

## Links

- [Managing Secrets in Kubernetes](https://www.weave.works/blog/managing-secrets-in-kubernetes)
- [Operator SDK](https://github.com/operator-framework/operator-sdk)
- [Vault](https://www.vaultproject.io)
