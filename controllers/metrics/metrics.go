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
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(VaultSecretsReconciliationsTotal)
}
