# Vault Secrets Operator

| Value | Description | Default |
| ----- | ----------- | ------- |
| `replicaCount` | Number of replications which should be created. | `1` |
| `deploymentStrategy` | Deployment strategy which should be used. | `{}` |
| `image.repository` | The repository of the Docker image. | `ricoberger/vault-secrets-operator` |
| `image.tag` | The tag of the Docker image which should be used. | `v1.23.1` |
| `image.digest` | The image digest of the Docker image which should be used | `""` |
| `image.pullPolicy` | The pull policy for the Docker image, | `IfNotPresent` |
| `image.volumeMounts` | Mount additional volumns to the container. | `[]` |
| `imagePullSecrets` | Secrets which can be used to pull the Docker image. | `[]` |
| `nameOverride` | Expand the name of the chart. | `""` |
| `fullnameOverride` | Override the name of the app. | `""` |
| `args` | Set the arguments for the operator. | `["-leader-elect"]` |
| `environmentVars` | Pass environment variables from a secret to the containers. This must be used if you use the Token auth method of Vault. | `[]` |
| `vault.address` | The address where Vault listen on (e.g. `http://vault.example.com`). | `"http://vault:8200"` |
| `vault.authMethod` | The authentication method, which should be used by the operator. Can by `token` ([Token auth method](https://www.vaultproject.io/docs/auth/token.html)), `aws` ([AWS auth method](https://www.vaultproject.io/docs/auth/aws)), `gcp` ([GCP auth method](https://www.vaultproject.io/docs/auth/gcp)), `kubernetes` ([Kubernetes auth method](https://www.vaultproject.io/docs/auth/kubernetes.html)), or `approle` ([AppRole auth method](https://www.vaultproject.io/docs/auth/approle)). | `token` |
| `vault.tokenPath` | Path to file with the Vault token if the used auth method is `token`. Can be used to read the token from a file and not from the  `VAULT_TOKEN` environment variable. | `""` |
| `vault.kubernetesPath` | If the Kubernetes auth method is used, this is the path where the Kubernetes auth method is enabled. | `auth/kubernetes` |
| `vault.kubernetesRole` | The name of the role which is configured for the Kubernetes auth method. | `vault-secrets-operator` |
| `vault.awsPath` | If the AWS auth method is used, this is the path where the AWS auth method is enabled. | `auth/aws` |
| `vault.awsAuthType` | If the AWS auth method is used, this is the auth type AWS auth method should use. `ec2` or `iam` | `iam` |
| `vault.awsRole` | The name of the role which is configured for the AWS auth method. | `vault-secrets-operator` |
| `vault.gcpPath` | If the GCP auth method is used, this is the path where the GCP auth method is enabled. | `auth/gcp` |
| `vault.gcpAuthType` | If the GCP auth method is used, this is the auth type GCP auth method should use. `gce` or `iam` | `iam` |
| `vault.gcpServiceAccountEmail` | If the GCP auth method and iam auth type are used, this is the email of the service account to authenticate as. If not specified, vault-secrets-operator will try to deduce it from GCP metadata server | `""` |
| `vault.gcpRole` | The name of the role which is configured for the GCP auth method. | `vault-secrets-operator` |
| `vault.appRolePath` | If the AppRole auth method is used, this is the path where the AppRole auth method is enabled. | `auth/approle` |
| `vault.reconciliationTime` | The time after which the reconcile function for the CR is rerun. If the value is 0, automatic reconciliation is skipped. | `0` |
| `vault.namespaces` | Comma serpareted list of namespaces the operator will watch. If empty the operator will watch all namespaces. | `""` |
| `rbac.create` | Create RBAC object, enable (Cluster)Role and (Cluster)Role binding creation. | `true` |
| `rbac.createrole` | Finetune RBAC, enable or disable (Cluster)Role creation. NOTE: ignored when `rbac.create` is not `true`. | `true` |
| `rbac.namespaced` | Deploy in isolated namespace. Creates RoleBinding instead of a ClusterRoleBinding | `false` |
| `serviceAccount.create` | Create the service account. | `true` |
| `serviceAccount.name` | The name of the service account, which should be created/used by the operator. | `vault-secrets-operator` |
| `serviceAccount.annotations` | Annotations to be added to service account. | `{}` |
| `serviceAccount.createSecret` | Create a secret for the service account. | `true` |
| `podAnnotations` | Annotations for vault-secrets-operator pod(s). | `{}` |
| `podSecurityContext`: | Security context policies to add to the operator pod. | `{}` |
| `securityContext`: | Security context policies to add to the containers. | `{}` |
| `podLabels` | Additional labels for the vault-secrets-operator pod(s). | `{}` |
| `testPodAnnotations` | Annotations for vault-secrets-operator-test-connection pod. | `{}` |
| `testPodLabels` | Additional labels for the vault-secrets-operator-test-connection pod. | `{}` |
| `resources` | Set resources for the operator. | `{}` |
| `volumes` | Provide additional volumns for the container. | `[]` |
| `nodeSelector` | Set a node selector. | `{}` |
| `tolerations` | Set tolerations. | `[]` |
| `serviceMonitor.enabled` | Enable the creation of a ServiceMonitor for the Prometheus Operator. | `false` |
| `serviceMonitor.labels` | Additional labels which should be set for the ServiceMonitor. | `{}` |
| `serviceMonitor.interval` | Scrape interval. | `10s` |
| `serviceMonitor.scrapeTimeout` | Scrape timeout. | `10s` |
| `serviceMonitor.honorLabels` | Honor labels option. | `true` |
| `serviceMonitor.relabelings` | Additional relabeling config for the ServiceMonitor. | `[]` |
| `priorityClassName` | Optionally attach priority class to pod spec. | `null` |
| `dnsConfig` | Optionally attach dnsConfig to pod spec and test pod spec. | `null` |
| `dnsPolicy` | Optionally specify the dnsPolicy in pod spec and test pod spec. | `null` |
