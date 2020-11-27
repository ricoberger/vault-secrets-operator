#!/usr/bin/env bash

set -o errexit

# Create registry container unless it already exists
reg_name='kind-registry'
reg_port='5000'
running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  docker run \
    -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" \
    registry:2
fi

# Create a cluster with the local registry enabled in containerd
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_name}:${reg_port}"]
EOF

# Connect the registry to the cluster network (the network may already be connected)
docker network connect "kind" "${reg_name}" || true

# Document the local registry
# https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

# Build the image for the operator and push the image to our local registry
docker build . -t localhost:5000/vault-secrets-operator:test
docker push localhost:5000/vault-secrets-operator:test

kubectl create ns vault
kubectl create ns vault-secrets-operator

# Install Vault in the cluster and create a new secret engine for the operator
helm repo add hashicorp https://helm.releases.hashicorp.com
helm upgrade --install vault hashicorp/vault --namespace=vault --set server.dev.enabled=true --set injector.enabled=false

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

# Install the operator via the Helm chart and enable the Kubernetes authentication method for the operator
helm upgrade --install vault-secrets-operator ./charts/vault-secrets-operator --namespace=vault-secrets-operator --set vault.address="http://vault.vault.svc.cluster.local:8200" --set vault.authMethod="kubernetes" --set image.repository="localhost:5000/vault-secrets-operator" --set image.tag="test"

export VAULT_SECRETS_OPERATOR_NAMESPACE=$(kubectl get sa --namespace=vault-secrets-operator vault-secrets-operator -o jsonpath="{.metadata.namespace}")
export VAULT_SECRET_NAME=$(kubectl get sa --namespace=vault-secrets-operator vault-secrets-operator -o jsonpath="{.secrets[*]['name']}")
export SA_JWT_TOKEN=$(kubectl get secret --namespace=vault-secrets-operator $VAULT_SECRET_NAME -o jsonpath="{.data.token}" | base64 --decode; echo)
export SA_CA_CRT=$(kubectl get secret --namespace=vault-secrets-operator $VAULT_SECRET_NAME -o jsonpath="{.data['ca\.crt']}" | base64 --decode; echo)
export K8S_HOST=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
env | grep -E 'VAULT_SECRETS_OPERATOR_NAMESPACE|VAULT_SECRET_NAME|SA_JWT_TOKEN|SA_CA_CRT|K8S_HOST'
vault auth enable kubernetes
vault write auth/kubernetes/config token_reviewer_jwt="$SA_JWT_TOKEN" kubernetes_host="https://kubernetes.default.svc" kubernetes_ca_cert="$SA_CA_CRT"
vault write auth/kubernetes/role/vault-secrets-operator bound_service_account_names="vault-secrets-operator" bound_service_account_namespaces="$VAULT_SECRETS_OPERATOR_NAMESPACE" policies=vault-secrets-operator ttl=24h

# Create a VaultSecret for the "helloworld" secret from Vault
cat <<EOF | kubectl apply -f -
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: helloworld
spec:
  path: kvv2/helloworld
  type: Opaque
EOF

# Delete the operator Pod to use the newly configured Service Account and check if the operator created a Kubernetes secret for our example
kubectl wait pod --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator --for=condition=Ready --timeout=180s
kubectl delete pod --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator
kubectl wait pod --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator --for=condition=Ready --timeout=180s
sleep 10s
kubectl get secret helloworld -o yaml
kubectl  logs --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator
