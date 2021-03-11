package controllers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/template"
	"time"

	ricobergerdev1alpha1 "github.com/ricoberger/vault-secrets-operator/api/v1alpha1"
	"github.com/ricoberger/vault-secrets-operator/jks"
	"github.com/ricoberger/vault-secrets-operator/vault"

	"github.com/Masterminds/sprig"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VaultSecretReconciler reconciles a VaultSecret object
type VaultSecretReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ricoberger.de,resources=vaultsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ricoberger.de,resources=vaultsecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ricoberger.de,resources=vaultsecrets/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core;coordination.k8s.io,resources=configmaps;leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *VaultSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("vaultsecret", req.NamespacedName)

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

	var errInvalidSecretData = fmt.Errorf("invalid secret data")
	var errSecretIsNil = fmt.Errorf("secret is nil")
	var errSecretPathIsNil = fmt.Errorf("path field is nil")

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
			return ctrl.Result{}, err
		}
		data, err = vaultClient.GetSecret(instance.Spec.SecretEngine, instance.Spec.Path, instance.Spec.Keys, instance.Spec.Version, instance.Spec.IsBinary, instance.Spec.VaultNamespace)
		if err != nil {
			// Error while getting the secret from Vault - requeue the request.
			if instance.Spec.Jks.Type == "truststore" && (reflect.DeepEqual(err, errSecretIsNil) || reflect.DeepEqual(err, errInvalidSecretData) || reflect.DeepEqual(err, errSecretPathIsNil)) {
				log.Info("Could not get secret from vault, but moving on to create/update default truststore...")
				// make an empty slice, so that creating truststore won't fail
				data = make(map[string][]byte)
			} else {
				log.Error(err, "Could not get secret from vault")
				return ctrl.Result{}, err
			}
		}
	} else {
		log.Info("Use shared client to get secret from Vault")
		vaultClient = vault.SharedClient
		if vaultClient == nil {
			err = fmt.Errorf("shared client not initilized and vaultRole property missing")
			log.Error(err, "Could not get secret from Vault")
			return ctrl.Result{}, err
		}

		data, err = vault.SharedClient.GetSecret(instance.Spec.SecretEngine, instance.Spec.Path, instance.Spec.Keys, instance.Spec.Version, instance.Spec.IsBinary, instance.Spec.VaultNamespace)
		if err != nil {
			// Error while getting the secret from Vault - requeue the request.
			if instance.Spec.Jks.Type == "truststore" && (reflect.DeepEqual(err, errSecretIsNil) || reflect.DeepEqual(err, errInvalidSecretData) || reflect.DeepEqual(err, errSecretPathIsNil)) {
				log.Info("Could not get secret from vault, but moving on to create/update default truststore...")
				// make an empty slice, so that creating truststore won't fail
				data = make(map[string][]byte)
			} else {
				log.Error(err, "Could not get secret from vault")
				return ctrl.Result{}, err
			}
		}
	}

	// Define a new Secret object
	secret, err := newSecretForCR(instance, data)
	if err != nil {
		// Error while creating the Kubernetes secret - requeue the request.
		log.Error(err, "Could not create Kubernetes secret")
		return ctrl.Result{}, err
	}

	// Set VaultSecret instance as the owner and controller
	err = ctrl.SetControllerReference(instance, secret, r.Scheme)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if this Secret already exists
	found := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)

		data, err := getDataBasedOnSecretCategory(instance, secret, secret.Data, vaultClient)
		if err != nil {
			return ctrl.Result{}, err
		}
		secret.Data = data
		err = r.Create(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Secret created successfully - requeue only if no version is specified
		return reconcileResult, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Secret already exists, update the secret
	// Merge -> Checks the existing data keys and merge them into the updated secret
	// Replace -> Do not check the data keys and replace the secret

	data, err = getDataBasedOnSecretCategory(instance, found, secret.Data, vaultClient)
	if err != nil {
		return ctrl.Result{}, err
	}
	secret.Data = data

	if instance.Spec.ReconcileStrategy == "Merge" {
		secret = mergeSecretData(secret, found)

		log.Info("Updating a Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		err = r.Update(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Updating a Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		err = r.Update(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Secret updated successfully - requeue only if no version is specified
	return reconcileResult, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VaultSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ricobergerdev1alpha1.VaultSecret{}).
		Owns(&corev1.Secret{}).
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

	// We need to exclude some functions for security reasons and proper working of the operator, don't use TxtFuncMap:
	// - no environment-variable related functions to prevent secrets from accessing the VAULT environment variables
	// - no filesystem functions? Directory functions don't actually allow access to the FS, so they're OK.
	// - no other non-idempotent functions like random and crypto functions
	funcmap := sprig.HermeticTxtFuncMap()
	delete(funcmap, "genPrivateKey")
	delete(funcmap, "genCA")
	delete(funcmap, "genSelfSignedCert")
	delete(funcmap, "genSignedCert")
	delete(funcmap, "htpasswd") // bcrypt strings contain salt

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

// newSecretForCR returns a secret with the same name/namespace as the CR. The secret will include all labels and
// annotations from the CR.
func newSecretForCR(cr *ricobergerdev1alpha1.VaultSecret, data map[string][]byte) (*corev1.Secret, error) {
	labels := map[string]string{
		"created-by": "vault-secrets-operator",
	}
	for k, v := range cr.ObjectMeta.Labels {
		labels[k] = v
	}

	annotations := map[string]string{}
	for k, v := range cr.ObjectMeta.Annotations {
		annotations[k] = v
	}

	if cr.Spec.Templates != nil {
		newdata := make(map[string][]byte)
		for k, v := range cr.Spec.Templates {
			templated, err := runTemplate(cr, v, data)
			if err != nil {
				return nil, fmt.Errorf("Template ERROR: %w", err)
			}
			newdata[k] = templated
		}
		data = newdata
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.Name,
			Namespace:   cr.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Data: data,
		Type: cr.Spec.Type,
	}, nil
}

func mergeSecretData(new, found *corev1.Secret) *corev1.Secret {
	for key, value := range found.Data {
		if _, ok := new.Data[key]; !ok {
			new.Data[key] = value
		}
	}

	return new
}

func getDataBasedOnSecretCategory(cr *ricobergerdev1alpha1.VaultSecret, secret *corev1.Secret, data map[string][]byte, client *vault.Client) (map[string][]byte, error) {
	if cr.Spec.Jks.Type == "keystore" || cr.Spec.Jks.Type == "truststore" {
		var jksName string
		jksType := cr.Spec.Jks.Type

		if cr.Spec.Jks.Name != "" {
			jksName = cr.Spec.Jks.Name
			if !strings.HasSuffix(jksName, ".jks") {
				jksName += ".jks"
			}
		} else {
			jksName = jksType + ".jks"
		}

		jksPassName := strings.Replace(jksName, ".jks", "Pass", 1)

		keystore, keystorePass, err := jks.GetKeystoreFromSecret(secret, data, jksName, jksPassName, jksType, client)
		if err != nil {
			return nil, err
		}

		// replace 'data' which is secret key map from vault, with keystore
		// and keystore pass map, to be placed in k8s secret.
		newData := make(map[string][]byte)
		newData[jksName] = keystore
		newData[jksPassName] = []byte(keystorePass)
		data = newData
	}
	return data, nil

}
