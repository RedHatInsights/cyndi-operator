package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"
)

var (
	hostCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cyndi_hosts_total",
		Help: "Total number of hosts in the given table in the application database",
	}, []string{"app"})

	inconsistencyRatio = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cyndi_inconsistency_ratio",
		Help: "The ratio of inconsistency of data between the source database and the application replica",
	}, []string{"app"})

	inconsistencyAbsolute = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cyndi_inconsistency_total",
		Help: "The total number of hosts that are not consistent with the origin",
	}, []string{"app"})

	inconsistencyThreshold = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cyndi_inconsistency_threshold",
		Help: "The threshold of inconsistency below which the pipeline is considered valid",
	}, []string{"app"})

	validationFailedCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "cyndi_validation_failed_total",
		Help: "The number of validation iterations that failed",
	}, []string{"app"})

	refreshCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "cyndi_refresh_total",
		Help: "The number of times this pipeline has been refreshed",
	}, []string{"app", "reason"})
)

type RefreshReason string

const (
	REFRESH_INVALID_PIPELINE RefreshReason = "invalid"
	REFRESH_STATE_DEVIATION  RefreshReason = "deviation"
)

func Init() {
	metrics.Registry.MustRegister(hostCount, inconsistencyRatio, inconsistencyAbsolute, inconsistencyThreshold, validationFailedCount, refreshCount)
}

func InitLabels(instance *cyndi.CyndiPipeline) {
	appName := instance.Spec.AppName
	hostCount.WithLabelValues(appName)
	inconsistencyThreshold.WithLabelValues(appName)
	inconsistencyRatio.WithLabelValues(appName)
	inconsistencyAbsolute.WithLabelValues(appName)
	validationFailedCount.WithLabelValues(appName)
	refreshCount.WithLabelValues(appName, string(REFRESH_INVALID_PIPELINE))
	refreshCount.WithLabelValues(appName, string(REFRESH_STATE_DEVIATION))
}

func AppHostCount(instance *cyndi.CyndiPipeline, value int64) {
	hostCount.WithLabelValues(instance.Spec.AppName).Set(float64(value))
}

func ValidationFinished(instance *cyndi.CyndiPipeline, threshold int64, ratio float64, inconsistentTotal int64, isValid bool) {
	inconsistencyThreshold.WithLabelValues(instance.Spec.AppName).Set(float64(threshold) / 100)
	inconsistencyRatio.WithLabelValues(instance.Spec.AppName).Set(ratio)
	inconsistencyAbsolute.WithLabelValues(instance.Spec.AppName).Set(float64(inconsistentTotal))

	if !isValid {
		validationFailedCount.WithLabelValues(instance.Spec.AppName).Inc()
	}
}

func PipelineRefreshed(instance *cyndi.CyndiPipeline, reason RefreshReason) {
	refreshCount.WithLabelValues(instance.Spec.AppName, string(reason)).Inc()
}
