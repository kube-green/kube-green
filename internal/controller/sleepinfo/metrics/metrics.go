package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type Metrics struct {
	CurrentSleepInfo *prometheus.GaugeVec
}

func SetupMetricsOrDie(prefix string) Metrics {
	sleepInfoMetrics := Metrics{
		CurrentSleepInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prefix,
			Name:      "current_sleepinfo",
			Help:      "Info about SleepInfo resource",
		}, []string{"name", "namespace"}),
	}
	return sleepInfoMetrics
}

func (customMetrics Metrics) MustRegister(registry metrics.RegistererGatherer) Metrics {
	registry.MustRegister(
		customMetrics.CurrentSleepInfo,
	)
	return customMetrics
}
