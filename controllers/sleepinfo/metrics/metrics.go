package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type Metrics struct {
	SleepWorkloadTotal        *prometheus.CounterVec
	CurrentSleepInfo          *prometheus.GaugeVec
	CurrentPermanentSleepInfo *prometheus.GaugeVec
}

func SetupMetricsOrDie(prefix string) Metrics {
	sleepInfoMetrics := Metrics{
		SleepWorkloadTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: prefix,
			Name:      "sleep_workload_total",
			Help:      "Total number of workload stopped by the controller",
		}, []string{"resource_type", "namespace"}),
		CurrentSleepInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prefix,
			Name:      "current_sleepinfo",
			Help:      "Info about SleepInfo resource",
		}, []string{"name", "namespace"}),
		CurrentPermanentSleepInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prefix,
			Name:      "current_permanent_sleepinfo",
			Help:      "Number of the currently SleepInfo resource without wakeUpAt set",
		}, []string{"name", "namespace"}),
	}
	return sleepInfoMetrics
}

func (customMetrics Metrics) MustRegister(registry metrics.RegistererGatherer) Metrics {
	registry.MustRegister(
		customMetrics.SleepWorkloadTotal,
		customMetrics.CurrentSleepInfo,
		customMetrics.CurrentPermanentSleepInfo,
	)
	return customMetrics
}
