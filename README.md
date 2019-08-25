# Vault Secrets Operator

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
