package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type Metrics struct {
	SleepWorkloadTotal *prometheus.CounterVec
	SleepInfoInfo      *prometheus.GaugeVec
}

func SetupMetricsOrDie(prefix string) Metrics {
	sleepInfoMetrics := Metrics{
		SleepWorkloadTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: prefix,
			Name:      "sleep_workload_total",
			Help:      "Total number of workload stopped by the controller",
		}, []string{"resource_type", "namespace"}),
		SleepInfoInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prefix,
			Name:      "sleepinfo_info",
			Help:      "Info about SleepInfo resource",
		}, []string{"name", "namespace"}),
	}
	return sleepInfoMetrics
}

func (customMetrics Metrics) MustRegister(registry metrics.RegistererGatherer) Metrics {
	registry.MustRegister(
		customMetrics.SleepWorkloadTotal,
		customMetrics.SleepInfoInfo,
	)
	return customMetrics
}

func getHour(n int) float64 {
	return time.Duration(n * int(time.Hour)).Seconds()
}
