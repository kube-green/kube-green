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

	m.CurrentSleepInfo.With(prometheus.Labels{
		"name":      "test_name",
		"namespace": "test_namespace",
	}).Set(1)

	return m
}

func TestMetrics(t *testing.T) {
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
}

func TestSetupMetricsAndRegister(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	getAndUseMetrics().MustRegister(registry)

	count, err := testutil.GatherAndCount(registry)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
