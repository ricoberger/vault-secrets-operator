# Vault Secrets Operator

The **Vault Secrets Operator** creates a Kubernetes secret from a Vault secret. The idea behind the Vault Secrets Operator is to manage secrets in Kubernetes using a secure GitOps based workflow. The Vault Secrets Operator reads a Vault secret from the defined path in a CR and creates a Kubernetes secret of the also defined type. The Operator supports the **Token Auth Method** and the **KV Secrets Engine - Version 1** from Vault.

> **Note:** This is not production ready yet. The Vault Secrets Operator is currently a test to get more familiar with the Operator SDK and Vault.

## Development

After modifying the `*_types.go` file always run the following command to update the generated code for that resource type:

```sh
operator-sdk generate k8s
```

To update the OpenAPI validation section in the CRD `deploy/crds/cache_v1alpha1_memcached_crd.yaml`, run the following command.

```sh
operator-sdk generate openapi
```

Create the Custom Resource Definition for the Vault Secrets Operator and the example Custom Resource:

```sh
kubectl apply -f deploy/crds/ricoberger_v1alpha1_vaultsecret_crd.yaml
kubectl apply -f deploy/crds/ricoberger_v1alpha1_vaultsecret_cr.yaml
```

Set the name of the operator in an environment variable:

```sh
export OPERATOR_NAME=vault-secrets-operator
```

Set the address and a token for Vault:

```sh
export VAULT_ADDRESS=
export VAULT_TOKEN=
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
