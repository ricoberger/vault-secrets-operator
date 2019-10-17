# Vault Secrets Operator

| Value | Description | Default |
| ----- | ----------- | ------- |
| `replicaCount` | Number of replications which should be created. | `1` |
| `deploymentStrategy` | Deployment strategy which should be used. | `{}` |
| `image.repository` | The repository of the Docker image. | `ricoberger/vault-secrets-operator` |
| `image.tag` | The tag of the Docker image which should be used. | `1.3.1` |
| `image.pullPolicy` | The pull policy for the Docker image, | `IfNotPresent` |
| `image.args` | Command-line arguments which should be passed to the container. This can be used to configure the logging. | `[]` |
| `image.volumeMounts` | Mount additional volumns to the container. | `[]` |
| `imagePullSecrets` | Secrets which can be used to pull the Docker image. | `[]` |
| `nameOverride` | Expand the name of the chart. | `""` |
| `fullnameOverride` | Override the name of the app. | `""` |
| `environmentVars` | Pass environment variables from a secret to the containers. This must be used if you use the Token auth method of Vault. | `[]` |
| `vault.address` | The address where Vault listen on (e.g. `http://vault.example.com`). | `"http://vault:8200"` |
| `vault.authMethod` | The authentication method, which should be used by the operator. Can by `token` ([Token auth method](https://www.vaultproject.io/docs/auth/token.html)) or `kubernetes` ([Kubernetes auth method](https://www.vaultproject.io/docs/auth/kubernetes.html)). | `token` |
| `vault.tokenPath` | Path to file with the Vault token if the used auth method is `token`. Can be used to read the token from a file and not from the  `VAULT_TOKEN` environment variable. | `""` |
| `vault.kubernetesPath` | If the Kubernetes auth method is used, this is the path where the Kubernetes auth method is enabled. | `auth/kubernetes` |
| `vault.kubernetesRole` | The name of the role which is configured for the Kubernetes auth method. | `vault-secrets-operator` |
| `vault.reconciliationTime` | The time after which the reconcile function for the CR is rerun. If the value is 0, automatic reconciliation is skipped. | `0` |
| `crd.create` | Create the custom resource definition. | `true` |
| `rbac.create` | Create the cluster role and cluster role bindings. | `true` |
| `serviceAccount.create` | Create the service account. | `true` |
| `serviceAccount.name` | The name of the service account, which should be created/used by the operator. | `vault-secrets-operator` |
| `service.type` | Type of the service, whiche should be created. | `ClusterIP` |
| `service.metricsPort` | Port for the metrics. | `8383` |
| `service.operatorMetricsPort` | Port for the operator metrics. | `8686` |
| `resources` | Set resources for the operator. | `{}` |
| `volumes` | Provide additional volumns for the container. | `[]` |
| `nodeSelector` | Set a node selector. | `{}` |
| `tolerations` | Set tolerations. | `[]` |
| `affinity` | Set the affinity. | `{}` |
