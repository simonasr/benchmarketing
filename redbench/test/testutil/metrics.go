package testutil

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// CapturePrometheusMetrics captures a snapshot of current metrics values.
// This utility function is useful across multiple test files for metrics verification.
func CapturePrometheusMetrics(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	metrics := make(map[string]float64)

	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Logf("Failed to gather metrics: %v", err)
		return metrics
	}

	for _, mf := range metricFamilies {
		for _, metric := range mf.GetMetric() {
			name := mf.GetName()

			// Add labels to create unique metric names
			if len(metric.GetLabel()) > 0 {
				for _, label := range metric.GetLabel() {
					name += "_" + label.GetName() + "_" + label.GetValue()
				}
			}

			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				if metric.Counter != nil {
					metrics[name] = metric.Counter.GetValue()
				}
			case dto.MetricType_GAUGE:
				if metric.Gauge != nil {
					metrics[name] = metric.Gauge.GetValue()
				}
			case dto.MetricType_HISTOGRAM:
				if metric.Histogram != nil {
					metrics[name+"_count"] = float64(metric.Histogram.GetSampleCount())
					metrics[name+"_sum"] = metric.Histogram.GetSampleSum()
				}
			}
		}
	}

	return metrics
}
