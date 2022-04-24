package metrics

import (
	"bytes"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func getMetrics() Metrics {
	return SetupMetricsOrDie("test_prefix")
}

func TestMetrics(t *testing.T) {
	t.Run("ActualSleepReplicasTotal", func(t *testing.T) {
		m := getMetrics()
		m.ActualSleepReplicasTotal.WithLabelValues("deployment", "test_namespace").Add(1)
		m.ActualSleepReplicasTotal.WithLabelValues("cronjob", "test_namespace").Add(1)
		m.ActualSleepReplicasTotal.WithLabelValues("deployment", "another_namespace").Add(13)

		prob, err := testutil.CollectAndLint(m.ActualSleepReplicasTotal)
		require.NoError(t, err)
		require.Nil(t, prob)

		buf := bytes.NewBufferString(`
		# HELP test_prefix_actual_sleep_replicas Actual number of replicas stopped by the controller
		# TYPE test_prefix_actual_sleep_replicas gauge
		test_prefix_actual_sleep_replicas{namespace="test_namespace",resource_type="deployment"} 1
		test_prefix_actual_sleep_replicas{namespace="test_namespace",resource_type="cronjob"} 1
		test_prefix_actual_sleep_replicas{namespace="another_namespace",resource_type="deployment"} 13
		`)
		require.NoError(t, testutil.CollectAndCompare(m.ActualSleepReplicasTotal, buf))
	})

	t.Run("TotalSleepWorkload", func(t *testing.T) {
		m := getMetrics()
		m.TotalSleepWorkload.WithLabelValues("deployment", "test_namespace").Add(2)

		prob, err := testutil.CollectAndLint(m.TotalSleepWorkload)
		require.NoError(t, err)
		require.Nil(t, prob)

		buf := bytes.NewBufferString(`
		# HELP test_prefix_sleep_workload_total Total number of workload stopped by the controller
		# TYPE test_prefix_sleep_workload_total counter
		test_prefix_sleep_workload_total{namespace="test_namespace",resource_type="deployment"} 2
		`)
		require.NoError(t, testutil.CollectAndCompare(m.TotalSleepWorkload, buf))
	})

	t.Run("SleepInfoInfo", func(t *testing.T) {
		m := getMetrics()
		m.SleepInfoInfo.With(prometheus.Labels{
			"namespace":      "test_namespace",
			"is_wake_up_set": "true",
			"deployments":    "true",
			"cronjobs":       "false",
		}).Inc()

		prob, err := testutil.CollectAndLint(m.SleepInfoInfo)
		require.NoError(t, err)
		require.Nil(t, prob)

		buf := bytes.NewBufferString(`
		# HELP test_prefix_sleepinfo_info_total Info about SleepInfo resource
		# TYPE test_prefix_sleepinfo_info_total counter
		test_prefix_sleepinfo_info_total{cronjobs="false",deployments="true",is_wake_up_set="true",namespace="test_namespace"} 1
		`)
		require.NoError(t, testutil.CollectAndCompare(m.SleepInfoInfo, buf))
	})

	t.Run("SleepDurationSeconds", func(t *testing.T) {
		m := getMetrics()
		m.SleepDurationSeconds.With(prometheus.Labels{
			"namespace": "test_namespace",
		}).Observe(time.Hour.Seconds())
		m.SleepDurationSeconds.With(prometheus.Labels{
			"namespace": "test_namespace",
		}).Observe(20 * time.Hour.Seconds())

		prob, err := testutil.CollectAndLint(m.SleepDurationSeconds)
		require.NoError(t, err)
		require.Nil(t, prob)

		buf := bytes.NewBufferString(`
		# HELP test_prefix_sleep_duration_seconds Sleep duration in seconds with bucket 1h, 3h, 5h, 8h, 12h, 24h, +24h
		# TYPE test_prefix_sleep_duration_seconds histogram
		test_prefix_sleep_duration_seconds_bucket{namespace="test_namespace",le="3600"} 1
		test_prefix_sleep_duration_seconds_bucket{namespace="test_namespace",le="10800"} 1
		test_prefix_sleep_duration_seconds_bucket{namespace="test_namespace",le="18000"} 1
		test_prefix_sleep_duration_seconds_bucket{namespace="test_namespace",le="28800"} 1
		test_prefix_sleep_duration_seconds_bucket{namespace="test_namespace",le="43200"} 1
		test_prefix_sleep_duration_seconds_bucket{namespace="test_namespace",le="86400"} 2
		test_prefix_sleep_duration_seconds_bucket{namespace="test_namespace",le="+Inf"} 2
		test_prefix_sleep_duration_seconds_sum{namespace="test_namespace"} 75600
		test_prefix_sleep_duration_seconds_count{namespace="test_namespace"} 2
		`)
		require.NoError(t, testutil.CollectAndCompare(m.SleepDurationSeconds, buf))
	})
}
