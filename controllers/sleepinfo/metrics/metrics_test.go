package metrics

import (
	"bytes"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func getMetrics() Metrics {
	return SetupMetricsOrDie("test_prefix")
}

func getAndUseMetrics() Metrics {
	m := getMetrics()

	m.SleepWorkloadTotal.WithLabelValues("deployment", "test_namespace").Add(2)

	m.CurrentSleepInfo.With(prometheus.Labels{
		"name":      "test_name",
		"namespace": "test_namespace",
	}).Set(1)

	m.CurrentPermanentSleepInfo.With(prometheus.Labels{
		"name":      "test_name",
		"namespace": "test_namespace",
	}).Set(1)

	return m
}

func TestMetrics(t *testing.T) {
	t.Run("TotalSleepWorkload", func(t *testing.T) {
		m := getAndUseMetrics()

		prob, err := testutil.CollectAndLint(m.SleepWorkloadTotal)
		require.NoError(t, err)
		require.Nil(t, prob)

		buf := bytes.NewBufferString(`
		# HELP test_prefix_sleep_workload_total Total number of workload stopped by the controller
		# TYPE test_prefix_sleep_workload_total counter
		test_prefix_sleep_workload_total{namespace="test_namespace",resource_type="deployment"} 2
		`)
		require.NoError(t, testutil.CollectAndCompare(m.SleepWorkloadTotal, buf))
	})

	t.Run("CurrentSleepInfo", func(t *testing.T) {
		m := getAndUseMetrics()

		prob, err := testutil.CollectAndLint(m.CurrentSleepInfo)
		require.NoError(t, err)
		require.Nil(t, prob)

		buf := bytes.NewBufferString(`
		# HELP test_prefix_current_sleepinfo Info about SleepInfo resource
		# TYPE test_prefix_current_sleepinfo gauge
		test_prefix_current_sleepinfo{name="test_name",namespace="test_namespace"} 1
		`)
		require.NoError(t, testutil.CollectAndCompare(m.CurrentSleepInfo, buf))
	})

	t.Run("CurrentPermanentSleepInfo", func(t *testing.T) {
		m := getAndUseMetrics()

		prob, err := testutil.CollectAndLint(m.CurrentPermanentSleepInfo)
		require.NoError(t, err)
		require.Nil(t, prob)

		buf := bytes.NewBufferString(`
		# HELP test_prefix_current_permanent_sleepinfo Number of the currently SleepInfo resource without wakeUpAt set
		# TYPE test_prefix_current_permanent_sleepinfo gauge
		test_prefix_current_permanent_sleepinfo{name="test_name",namespace="test_namespace"} 1
		`)
		require.NoError(t, testutil.CollectAndCompare(m.CurrentPermanentSleepInfo, buf))
	})
}

func TestSetupMetricsAndRegister(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	getAndUseMetrics().MustRegister(registry)

	count, err := testutil.GatherAndCount(registry)
	require.NoError(t, err)
	require.Equal(t, 3, count)
}
