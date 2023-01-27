package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	VaultSecretsReconciliationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vaultsecrets_reconciliations_total",
			Help: "Total number of reconciliations",
		},
		[]string{"namespace", "name", "status"},
	)
	VaultSecretsReconciliationStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vaultsecrets_reconciliation_status",
			Help: "Reconciliation status (0 = failed and 1 = ok)",
		},
		[]string{"namespace", "name"},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(VaultSecretsReconciliationsTotal)
	metrics.Registry.MustRegister(VaultSecretsReconciliationStatus)
}
