package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	ricobergerdev1alpha1 "github.com/ricoberger/vault-secrets-operator/api/v1alpha1"
	"github.com/ricoberger/vault-secrets-operator/internal/controller"
	"github.com/ricoberger/vault-secrets-operator/internal/vault"

	// +kubebuilder:scaffold:imports

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	webhookserver "sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(ricobergerdev1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Create the API client for Vault and start the renew process for the
	// token in a new goroutine.
	err := vault.InitSharedClient()
	if err != nil {
		ctrl.Log.Error(err, "Could not create API client for Vault")
		os.Exit(1)
	}

	if vault.SharedClient != nil {
		if vault.SharedClient.PerformRenewToken() {
			go vault.SharedClient.RenewToken()
		}
	} else {
		ctrl.Log.Info("Shared client wasn't initialized, each secret must be use the vaultRole property")
	}

	watchNamespace, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "unable to get WatchNamespace, the manager will watch and manage resources in all namespaces")
	}

	labelSelector := os.Getenv("WATCH_NAMESPACE_LABEL_SELECTOR")

	// Normalize the comma-separated namespace list (e.g. "ns1, ns2").
	space := regexp.MustCompile(`\s+`)
	var namespaceList []string
	if watchNamespace != "" {
		for _, ns := range strings.Split(space.ReplaceAllString(watchNamespace, ""), ",") {
			if ns != "" {
				namespaceList = append(namespaceList, ns)
			}
		}
	}

	nsFilter, err := controller.NewNamespaceFilter(namespaceList, labelSelector)
	if err != nil {
		setupLog.Error(err, "invalid WATCH_NAMESPACE_LABEL_SELECTOR")
		os.Exit(1)
	}

	options := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhookserver.NewServer(webhookserver.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "vaultsecretsoperator.ricoberger.de",
	}

	// reconcilerFilter is non-nil only when a label selector is configured; in
	// that mode the cache is cluster-wide and the reconciler filters namespaces.
	var reconcilerFilter *controller.NamespaceFilter

	switch {
	case nsFilter.UsesLabelSelector():
		setupLog.Info("manager set up with namespace label selector",
			"namespaces", watchNamespace, "labelSelector", labelSelector)
		reconcilerFilter = nsFilter
	case len(namespaceList) > 0:
		// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
		setupLog.Info("manager set up with multiple namespaces", "namespaces", watchNamespace)

		namespaces := make(map[string]cache.Config, len(namespaceList))
		for _, ns := range namespaceList {
			namespaces[ns] = cache.Config{}
		}

		options.NewCache = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			opts.DefaultNamespaces = namespaces
			return cache.New(config, opts)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.VaultSecretReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		NamespaceFilter: reconcilerFilter,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VaultSecret")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	err = mgr.AddHealthzCheck("healthz", func(_ *http.Request) error {
		if vault.SharedClient == nil {
			return nil
		}

		return vault.SharedClient.GetHealth(10)
	})
	if err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	err = mgr.AddReadyzCheck("readyz", func(_ *http.Request) error {
		if vault.SharedClient == nil {
			return nil
		}

		return vault.SharedClient.GetHealth(5)
	})
	if err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getWatchNamespace returns the Namespace the operator should be watching for
// changes
func getWatchNamespace() (string, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch. An empty value means the operator
	// is running with cluster scope.
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	return ns, nil
}
