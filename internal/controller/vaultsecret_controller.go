package controller

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"text/template"
	"time"

	ricobergerdev1alpha1 "github.com/ricoberger/vault-secrets-operator/api/v1alpha1"
	"github.com/ricoberger/vault-secrets-operator/internal/validators"
	"github.com/ricoberger/vault-secrets-operator/internal/vault"

	"github.com/Masterminds/sprig/v3"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logr "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	conditionTypeSecretCreated     = "SecretCreated"
	conditionReasonFetchFailed     = "FetchFailed"
	conditionReasonCreated         = "Created"
	conditionReasonCreateFailed    = "CreateFailed"
	conditionReasonUpdated         = "Updated"
	conditionReasonUpdateFailed    = "UpdateFailed"
	conditionReasonMergeFailed     = "MergeFailed"
	conditionReasonInvalidResource = "InvalidResource"

	vaultsecretsFinalizer = "vaultsecrets.ricoberger.de/finalizer"
)

const (
	kvEngine  = "kv"
	pkiEngine = "pki"
)

var (
	vaultSecretsReconciliationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vaultsecrets_reconciliations_total",
			Help: "Total number of reconciliations",
		},
		[]string{"namespace", "name", "status"},
	)
	vaultSecretsReconciliationStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vaultsecrets_reconciliation_status",
			Help: "Reconciliation status (0 = failed and 1 = ok)",
		},
		[]string{"namespace", "name"},
	)
)

// VaultSecretReconciler reconciles a VaultSecret object
type VaultSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(vaultSecretsReconciliationsTotal)
	metrics.Registry.MustRegister(vaultSecretsReconciliationStatus)
}

// +kubebuilder:rbac:groups=ricoberger.de,resources=vaultsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ricoberger.de,resources=vaultsecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ricoberger.de,resources=vaultsecrets/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
// nolint:gocyclo
func (r *VaultSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logr.FromContext(ctx)

	// Set reconciliation if the vault-secret does not specify a version.
	reconcileResult := ctrl.Result{}
	if vault.ReconciliationTime > 0 {
		reconcileResult = ctrl.Result{
			RequeueAfter: time.Second * time.Duration(vault.ReconciliationTime),
		}
	}

	// Fetch the VaultSecret instance
	instance := &ricobergerdev1alpha1.VaultSecret{}

	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Check if the VaultSecret instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be
	// deleted.
	isVaultSecretMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isVaultSecretMarkedToBeDeleted {
		vaultSecretsReconciliationsTotal.DeleteLabelValues(instance.Namespace, instance.Name, string(metav1.ConditionTrue))
		vaultSecretsReconciliationsTotal.DeleteLabelValues(instance.Namespace, instance.Name, string(metav1.ConditionFalse))
		vaultSecretsReconciliationStatus.DeleteLabelValues(instance.Namespace, instance.Name)

		if controllerutil.ContainsFinalizer(instance, vaultsecretsFinalizer) {
			// Remove the vaultsecretsFinalizer. Once the finalizer is removed the object will be deleted.
			controllerutil.RemoveFinalizer(instance, vaultsecretsFinalizer)
			err = r.Update(ctx, instance)
			if err != nil {
				log.Error(err, "Failed to remove finalizer.")
				r.updateConditions(ctx, instance, conditionReasonUpdateFailed, err.Error(), metav1.ConditionFalse)
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	// Add the vaultsecretsFinalizer to the VaultSecret. The finilizer is needed
	// so that we can remove the metrics for a delete secret.
	if !controllerutil.ContainsFinalizer(instance, vaultsecretsFinalizer) {
		controllerutil.AddFinalizer(instance, vaultsecretsFinalizer)
		err := r.Update(ctx, instance)
		if err != nil {
			log.Error(err, "Failed to add finalizer.")
			r.updateConditions(ctx, instance, conditionReasonUpdateFailed, err.Error(), metav1.ConditionFalse)
			return ctrl.Result{}, err
		}
	}

	// Get secret from Vault.
	// If the VaultSecret contains the vaulRole property we are creating a new client with the specified Vault Role to
	// get the secret.
	// When the property isn't set we are using the shared client. It is also possible that the shared client is nil, so
	// that we have to check for this first. This could happen since we do not return an error when we initializing the
	// client during start up, to not require a default Vault Role.
	var data map[string][]byte

	var vaultClient *vault.Client

	if instance.Spec.VaultRole != "" {
		log.WithValues("vaultRole", instance.Spec.VaultRole).Info("Create client to get secret from Vault")
		vaultClient, err = vault.CreateClient(instance.Spec.VaultRole)
		if err != nil {
			// Error creating the Vault client - requeue the request.
			r.updateConditions(ctx, instance, conditionReasonFetchFailed, err.Error(), metav1.ConditionFalse)
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Use shared client to get secret from Vault")
		if vault.SharedClient == nil {
			err = fmt.Errorf("shared client not initialized and vaultRole property missing")
			log.Error(err, "Could not get secret from Vault")
			r.updateConditions(ctx, instance, conditionReasonFetchFailed, err.Error(), metav1.ConditionFalse)
			return ctrl.Result{}, err
		}

		vaultClient = vault.SharedClient
	}

	// If the `VAULT_RESTRICT_NAMESPACE` environment variable is set to `true` the operator should only reconcile
	// secrets which have the same Vault namespace configured as the operator (via the `VAULT_NAMESPACE` environment
	// variable).
	if restricted, rootNamespace := vaultClient.IsNamespaceRestricted(); restricted && instance.Spec.VaultNamespace != rootNamespace {
		log.Info("Ignore secret, since the operator is restricted to the another Vault namespace", "vaultNamespace", instance.Spec.VaultNamespace, "rootNamespace", rootNamespace)
		return ctrl.Result{}, nil
	}

	switch instance.Spec.SecretEngine {
	case "", kvEngine:
		data, err = vaultClient.GetSecret(instance.Spec.Path, instance.Spec.Keys, instance.Spec.Version, instance.Spec.IsBinary, instance.Spec.VaultNamespace)
		if err != nil {
			// Error while getting the secret from Vault - requeue the request.
			log.Error(err, "Could not get secret from vault")
			r.updateConditions(ctx, instance, conditionReasonFetchFailed, err.Error(), metav1.ConditionFalse)
			return ctrl.Result{}, err
		}

	case pkiEngine:
		if err := validators.ValidatePKI(instance); err != nil {
			log.Error(err, "Resource validation failed")
			r.updateConditions(ctx, instance, conditionReasonInvalidResource, err.Error(), metav1.ConditionFalse)
			return ctrl.Result{}, err
		}

		var expiration *time.Time
		data, expiration, err = vaultClient.GetCertificate(instance.Spec.Path, instance.Spec.Role, instance.Spec.EngineOptions)
		if err != nil {
			log.Error(err, "Could not get certificate from vault")
			r.updateConditions(ctx, instance, conditionReasonFetchFailed, err.Error(), metav1.ConditionFalse)
			return ctrl.Result{}, err
		}

		// Requeue before expiration
		log.Info(fmt.Sprintf("Certificate will expire on %s", expiration.String()))
		ra := time.Until(*expiration) - vaultClient.GetPKIRenew()
		if ra <= 0 {
			reconcileResult.RequeueAfter = 0 * time.Second
		} else {
			reconcileResult.RequeueAfter = ra
			log.Info(fmt.Sprintf("Certificate will be renewed on %s", time.Now().Add(ra).String()))
		}
	}

	// Define a new Secret object
	secret, err := newSecretForCR(instance, data)
	if err != nil {
		// Error while creating the Kubernetes secret - requeue the request.
		log.Error(err, "Could not create Kubernetes secret")
		r.updateConditions(ctx, instance, conditionReasonCreateFailed, err.Error(), metav1.ConditionFalse)
		return ctrl.Result{}, err
	}

	// Set VaultSecret instance as the owner and controller
	err = ctrl.SetControllerReference(instance, secret, r.Scheme)
	if err != nil {
		log.Error(err, "Could not set owner reference")
		r.updateConditions(ctx, instance, conditionReasonCreateFailed, err.Error(), metav1.ConditionFalse)
		return ctrl.Result{}, err
	}

	// Check if this Secret already exists
	found := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		err = r.Create(ctx, secret)
		if err != nil {
			log.Error(err, "Could not create secret")
			r.updateConditions(ctx, instance, conditionReasonCreateFailed, err.Error(), metav1.ConditionFalse)
			return ctrl.Result{}, err
		}

		// Secret created successfully - requeue only if no version is specified
		r.updateConditions(ctx, instance, conditionReasonCreated, "Secret was created", metav1.ConditionTrue)
		return reconcileResult, nil
	} else if err != nil {
		log.Error(err, "Could not create secret")
		r.updateConditions(ctx, instance, conditionReasonCreateFailed, err.Error(), metav1.ConditionFalse)
		return ctrl.Result{}, err
	}

	// Secret already exists, update the secret
	// Merge -> Checks the existing data keys and merge them into the updated secret
	// Replace -> Do not check the data keys and replace the secret
	if instance.Spec.ReconcileStrategy == "Merge" {
		secret = mergeSecretData(secret, found)

		if secret.Type == found.Type && reflect.DeepEqual(secret.Data, found.Data) && reflect.DeepEqual(secret.Labels, found.Labels) && reflect.DeepEqual(secret.Annotations, found.Annotations) && len(instance.Status.Conditions) == 1 && instance.Status.Conditions[0].Status == metav1.ConditionTrue {
			log.Info("Skip updating a Secret cause data no change", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		} else {
			log.Info("Updating a Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
			err = r.Update(ctx, secret)
			if err != nil {
				log.Error(err, "Could not update secret")
				r.updateConditions(ctx, instance, conditionReasonMergeFailed, err.Error(), metav1.ConditionFalse)
				return ctrl.Result{}, err
			}
			r.updateConditions(ctx, instance, conditionReasonUpdated, "Secret was updated", metav1.ConditionTrue)
		}
	} else {
		if secret.Type == found.Type && reflect.DeepEqual(secret.Data, found.Data) && reflect.DeepEqual(secret.Labels, found.Labels) && reflect.DeepEqual(secret.Annotations, found.Annotations) && len(instance.Status.Conditions) == 1 && instance.Status.Conditions[0].Status == metav1.ConditionTrue {
			// Skip updating the secret if there is not change to prevent
			// unnecessary Kubernetes API calls. We still increase the total
			// reconciliations metric and set the reconciliation status to 1, to
			// reflect that the reconciliation was successful, even if there was
			// no change.
			log.Info("Skip updating a Secret cause no change", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
			vaultSecretsReconciliationsTotal.WithLabelValues(instance.Namespace, instance.Name, string(metav1.ConditionTrue)).Inc()
			vaultSecretsReconciliationStatus.WithLabelValues(instance.Namespace, instance.Name).Set(1)
		} else {
			log.Info("Updating a Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
			err = r.Update(ctx, secret)
			if err != nil {
				log.Error(err, "Could not update secret")
				r.updateConditions(ctx, instance, conditionReasonUpdateFailed, err.Error(), metav1.ConditionFalse)
				return ctrl.Result{}, err
			}
			r.updateConditions(ctx, instance, conditionReasonUpdated, "Secret was updated", metav1.ConditionTrue)
		}
	}

	// Secret updated successfully - requeue only if no version is specified
	return reconcileResult, nil
}

func (r *VaultSecretReconciler) updateConditions(ctx context.Context, instance *ricobergerdev1alpha1.VaultSecret, reason, message string, status metav1.ConditionStatus) {
	vaultSecretsReconciliationsTotal.WithLabelValues(instance.Namespace, instance.Name, string(status)).Inc()
	if status == metav1.ConditionTrue {
		vaultSecretsReconciliationStatus.WithLabelValues(instance.Namespace, instance.Name).Set(1)
	} else {
		vaultSecretsReconciliationStatus.WithLabelValues(instance.Namespace, instance.Name).Set(0)
	}

	instance.Status.Conditions = []metav1.Condition{{
		Type:               conditionTypeSecretCreated,
		Status:             status,
		ObservedGeneration: instance.GetGeneration(),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
	}}

	err := r.Status().Update(ctx, instance)
	if err != nil {
		logr.FromContext(ctx).Error(err, "Could not update status")
	}
}

// ignorePredicate is used to ignore updates to CR status in which case metadata.Generation does not change.
func ignorePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *VaultSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ricobergerdev1alpha1.VaultSecret{}).
		Owns(&corev1.Secret{}).
		WithEventFilter(ignorePredicate()).
		Complete(r)
}

// Context provided to the templating engine

type templateVaultContext struct {
	Path    string
	Address string
}

type templateContext struct {
	Secrets     map[string]string
	Vault       templateVaultContext
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
}

// runTemplate executes a template with the given secrets map, filled with the Vault secrets
func runTemplate(cr *ricobergerdev1alpha1.VaultSecret, tmpl string, secrets map[string][]byte) ([]byte, error) {
	// Set up the context
	sd := templateContext{
		Secrets: make(map[string]string, len(secrets)),
		Vault: templateVaultContext{
			Path:    cr.Spec.Path,
			Address: os.Getenv("VAULT_ADDRESS"),
		},
		Namespace:   cr.Namespace,
		Labels:      cr.Labels,
		Annotations: cr.Annotations,
	}

	// For templating, these should all be strings, convert
	for k, v := range secrets {
		sd.Secrets[k] = string(v)
	}

	funcmap := templatingFunctions()

	tmplParser := template.New("data").Funcs(funcmap)

	// use other delimiters to prevent clashing with Helm templates
	tmplParser.Delims("{%", "%}")

	t, err := tmplParser.Parse(tmpl)
	if err != nil {
		return nil, err
	}

	var bout bytes.Buffer
	err = t.Execute(&bout, sd)
	if err != nil {
		return nil, err
	}

	return bout.Bytes(), nil
}

func templatingFunctions() template.FuncMap {
	// We need to exclude some functions for security reasons and proper working of the operator, don't use TxtFuncMap:
	// - no environment-variable related functions to prevent secrets from accessing the VAULT environment variables
	// - no filesystem functions? Directory functions don't actually allow access to the FS, so they're OK.
	// - no other non-idempotent functions like random and crypto functions
	funcmap := sprig.HermeticTxtFuncMap()

	// contain random inputs for cryptographic reasons
	delete(funcmap, "genPrivateKey")
	delete(funcmap, "genCA")
	delete(funcmap, "genCAWithKey")
	delete(funcmap, "genSelfSignedCert")
	delete(funcmap, "genSelfSignedCertWithKey")
	delete(funcmap, "genSignedCert")
	delete(funcmap, "genSignedCertWithKey")
	delete(funcmap, "htpasswd")
	delete(funcmap, "bcrypt")

	// plain random functions
	delete(funcmap, "randInt")

	return funcmap
}

// newSecretForCR returns a secret with the same name/namespace as the CR. The secret will include all labels and
// annotations from the CR.
func newSecretForCR(cr *ricobergerdev1alpha1.VaultSecret, data map[string][]byte) (*corev1.Secret, error) {
	if cr.Spec.Templates != nil {
		newdata := make(map[string][]byte)
		for k, v := range cr.Spec.Templates {
			templated, err := runTemplate(cr, v, data)
			if err != nil {
				return nil, fmt.Errorf("template ERROR: %w", err)
			}
			newdata[k] = templated
		}
		data = newdata
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.Name,
			Namespace:   cr.Namespace,
			Labels:      cr.Labels,
			Annotations: cr.Annotations,
		},
		Data: data,
		Type: cr.Spec.Type,
	}, nil
}

func mergeSecretData(newSecret, foundSecret *corev1.Secret) *corev1.Secret {
	for key, value := range foundSecret.Data {
		if _, ok := newSecret.Data[key]; !ok {
			newSecret.Data[key] = value
		}
	}

	return newSecret
}
