#!/usr/bin/env bash

set -o errexit

# Build the image for the operator and push the image to our local registry
docker build . -t localhost:5000/vault-secrets-operator:test
docker push localhost:5000/vault-secrets-operator:test

kubectl create ns vault
kubectl create ns vault-secrets-operator

# Install Vault in the cluster and create a new secret engine for the operator
helm repo add hashicorp https://helm.releases.hashicorp.com
helm upgrade --install vault hashicorp/vault --namespace=vault --version=0.31.0 --set server.dev.enabled=true --set injector.enabled=false --set server.image.tag="1.20.4"

sleep 10s
kubectl wait pod/vault-0 --namespace=vault  --for=condition=Ready --timeout=180s
kubectl port-forward --namespace vault vault-0 8200 &
sleep 10s

vault login -address="http://127.0.0.1:8200" root
vault secrets enable -path=kvv2 -version=2 -address="http://127.0.0.1:8200" kv
cat <<EOF | vault policy write -address="http://127.0.0.1:8200" vault-secrets-operator -
path "kvv2/data/*" {
  capabilities = ["read"]
}
EOF

# Enable Vault UserPass auth method
vault auth enable -address="http://127.0.0.1:8200" userpass

# Create new User
vault write -address="http://127.0.0.1:8200" auth/userpass/users/test password=1234 policies=vault-secrets-operator

# Set user and password
VAULT_USER="test"
VAULT_PASSWORD="1234"

cat <<EOF > ./vault-secrets-operator.env
VAULT_USER=$VAULT_USER
VAULT_PASSWORD=$VAULT_PASSWORD
EOF

kubectl create secret generic vault-secrets-operator \
  --namespace=vault-secrets-operator \
  --from-env-file=./vault-secrets-operator.env

cat <<EOF > ./values.yaml
vault:
  address: http://vault.vault.svc.cluster.local:8200
  authMethod: userpass
image:
  repository: localhost:5000/vault-secrets-operator
  tag: test
environmentVars:
  - name: VAULT_USER
    valueFrom:
      secretKeyRef:
        name: vault-secrets-operator
        key: VAULT_USER
  - name: VAULT_PASSWORD
    valueFrom:
      secretKeyRef:
        name: vault-secrets-operator
        key: VAULT_PASSWORD
serviceAccount:
  createSecret: false
EOF

helm upgrade --install vault-secrets-operator ./charts/vault-secrets-operator --namespace=vault-secrets-operator -f ./values.yaml

vault kv put -address="http://127.0.0.1:8200" kvv2/helloworld foo=bar

cat <<EOF | kubectl apply -f -
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: helloworld
spec:
  vaultRole: vault-secrets-operator
  path: kvv2/helloworld
  type: Opaque
EOF

kubectl wait pod --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator --for=condition=Ready --timeout=180s
sleep 10s
kubectl get secret helloworld -o yaml

kubectl logs --namespace=vault-secrets-operator -l app.kubernetes.io/instance=vault-secrets-operator
