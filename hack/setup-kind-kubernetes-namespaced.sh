#!/usr/bin/env bash

set -o errexit

# Build the image for the operator and push the image to our local registry
docker build . -t localhost:5000/vault-secrets-operator:test
docker push localhost:5000/vault-secrets-operator:test

kubectl create ns vault
kubectl create ns vault-secrets-operator

# Install Vault in the cluster and create a new secret engine for the operator
helm repo add hashicorp https://helm.releases.hashicorp.com
helm upgrade --install vault hashicorp/vault --namespace=vault --version=0.28.1 --set server.dev.enabled=true --set injector.enabled=false --set server.image.tag="1.17.2"

sleep 10s
kubectl wait pod/vault-0 --namespace=vault  --for=condition=Ready --timeout=180s
kubectl port-forward --namespace vault vault-0 8200 &
sleep 10s

vault login root
vault secrets enable -path=kvv2 -version=2 kv
cat <<EOF | vault policy write vault-secrets-operator -
path "kvv2/data/*" {
  capabilities = ["read"]
}
EOF
vault kv put kvv2/helloworld foo=bar

helm upgrade --install vault-secrets-operator ./charts/vault-secrets-operator --namespace=vault-secrets-operator --set vault.address="http://vault.vault.svc.cluster.local:8200" --set vault.authMethod="kubernetes" --set image.repository="localhost:5000/vault-secrets-operator" --set image.tag="test" --set rbac.namespaced="true"

export VAULT_SECRETS_OPERATOR_NAMESPACE=vault-secrets-operator
export VAULT_SECRET_NAME=vault-secrets-operator
export SA_JWT_TOKEN=$(kubectl get secret --namespace=vault-secrets-operator $VAULT_SECRET_NAME -o jsonpath="{.data.token}" | base64 --decode; echo)
export SA_CA_CRT=$(kubectl get secret --namespace=vault-secrets-operator $VAULT_SECRET_NAME -o jsonpath="{.data['ca\.crt']}" | base64 --decode; echo)
export K8S_HOST=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
env | grep -E 'VAULT_SECRETS_OPERATOR_NAMESPACE|VAULT_SECRET_NAME|SA_JWT_TOKEN|SA_CA_CRT|K8S_HOST'
vault auth enable kubernetes
vault write auth/kubernetes/config token_reviewer_jwt="$SA_JWT_TOKEN" kubernetes_host="https://kubernetes.default.svc" kubernetes_ca_cert="$SA_CA_CRT" issuer="https://kubernetes.default.svc.cluster.local"
vault write auth/kubernetes/role/vault-secrets-operator bound_service_account_names="vault-secrets-operator" bound_service_account_namespaces="$VAULT_SECRETS_OPERATOR_NAMESPACE" policies=vault-secrets-operator ttl=24h

cat <<EOF | kubectl apply -f -
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: helloworld
  namespace: vault-secrets-operator
spec:
  path: kvv2/helloworld
  type: Opaque
EOF

# Delete the operator Pod to use the newly configured Service Account and check if the operator created a Kubernetes secret for our example
kubectl wait pod --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator --for=condition=Ready --timeout=180s
kubectl delete pod --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator
kubectl wait pod --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator --for=condition=Ready --timeout=180s
sleep 10s
kubectl get secret --namespace=vault-secrets-operator helloworld -o yaml
kubectl logs --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator
