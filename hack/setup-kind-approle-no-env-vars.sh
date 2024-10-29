#!/usr/bin/env bash

set -o errexit

# Build the image for the operator and push the image to our local registry
docker build . -t localhost:5000/vault-secrets-operator:test
docker push localhost:5000/vault-secrets-operator:test

kubectl create ns vault
kubectl create ns vault-secrets-operator

# Install Vault in the cluster and create a new secret engine for the operator
helm repo add hashicorp https://helm.releases.hashicorp.com
helm upgrade --install vault hashicorp/vault --namespace=vault --version=0.28.1 --set server.dev.enabled=true --set injector.enabled=false --set server.image.tag="1.18.0"

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

# Enable Vault AppRole auth method
vault auth enable approle

# Create new AppRole
vault write auth/approle/role/vault-secrets-operator token_policies=vault-secrets-operator

# Get AppRole ID and secret ID
VAULT_ROLE_ID=$(vault read auth/approle/role/vault-secrets-operator/role-id -format=json | jq -r .data.role_id)
VAULT_SECRET_ID=$(vault write -f auth/approle/role/vault-secrets-operator/secret-id -format=json | jq -r .data.secret_id)

cat <<EOF > ./vault-secrets-operator.env
VAULT_ROLE_ID=$VAULT_ROLE_ID
VAULT_SECRET_ID=$VAULT_SECRET_ID
EOF

kubectl create secret generic vault-secrets-operator \
  --namespace=vault-secrets-operator \
  --from-env-file=./vault-secrets-operator.env

cat <<EOF > ./values.yaml
vault:
  address: http://vault.vault.svc.cluster.local:8200
  authMethod: approle
image:
  repository: localhost:5000/vault-secrets-operator
  tag: test
  volumeMounts:
    - name: vault-role-id
      mountPath: "/etc/vault/role/"
      readOnly: true
    - name: vault-secret-id
      mountPath: "/etc/vault/secret/"
      readOnly: true
environmentVars:
  - name: VAULT_ROLE_ID_PATH
    value: "/etc/vault/role/id"
  - name: VAULT_SECRET_ID_PATH
    value: "/etc/vault/secret/id"
volumes:
  - name: vault-role-id
    secret:
      secretName: vault-secrets-operator
      items:
        - key: VAULT_ROLE_ID
          path: "id"
  - name: vault-secret-id
    secret:
      secretName: vault-secrets-operator
      items:
        - key: VAULT_SECRET_ID
          path: "id"
serviceAccount:
  createSecret: false
EOF

helm upgrade --install vault-secrets-operator ./charts/vault-secrets-operator --namespace=vault-secrets-operator -f ./values.yaml

vault kv put kvv2/helloworld foo=bar

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
