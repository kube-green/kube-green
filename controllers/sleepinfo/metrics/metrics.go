package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type Metrics struct {
	SleepWorkloadTotal   *prometheus.CounterVec
	ActualSleepReplicas  *prometheus.GaugeVec
	SleepInfoInfo        *prometheus.CounterVec
	SleepDurationSeconds *prometheus.HistogramVec
}

func SetupMetricsOrDie(prefix string) Metrics {
	sleepInfoMetrics := Metrics{
		SleepWorkloadTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: prefix,
			Name:      "sleep_workload_total",
			Help:      "Total number of workload stopped by the controller",
		}, []string{"resource_type", "namespace"}),
		ActualSleepReplicas: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prefix,
			Name:      "actual_sleep_replicas",
			Help:      "Actual number of replicas stopped by the controller",
		}, []string{"resource_type", "namespace"}),
		SleepInfoInfo: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: prefix,
			Name:      "sleepinfo_info_total",
			Help:      "Info about SleepInfo resource",
		}, []string{"namespace", "is_wake_up_set"}),
		SleepDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: prefix,
			Name:      "sleep_duration_seconds",
			Help:      "Sleep duration in seconds with bucket 1h, 3h, 5h, 8h, 12h, 24h, +24h",
			Buckets:   []float64{getHour(1), getHour(3), getHour(5), getHour(8), getHour(12), getHour(24)},
		}, []string{"namespace"}),
	}
	return sleepInfoMetrics
}

func (customMetrics Metrics) MustRegister(registry metrics.RegistererGatherer) Metrics {
	registry.MustRegister(
		customMetrics.SleepWorkloadTotal,
		customMetrics.SleepDurationSeconds,
		customMetrics.ActualSleepReplicas,
		customMetrics.SleepInfoInfo,
	)
	return customMetrics
}

func getHour(n int) float64 {
	return time.Duration(n * int(time.Hour)).Seconds()
}
