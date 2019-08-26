# Vault Secrets Operator

The **Vault Secrets Operator** creates a Kubernetes secret from a Vault secret. The idea behind the Vault Secrets Operator is to manage secrets in Kubernetes using a secure GitOps based workflow. The Vault Secrets Operator reads a Vault secret from the defined path in a CR and creates a Kubernetes secret of the also defined type. The Operator supports the **Token Auth Method** and the **KV Secrets Engine - Version 1** from Vault.

## Usage

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

// TODO: Deploy the Vault Secrets Operator

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
