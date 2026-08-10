package outbox

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestPrometheusMetricsRecordsOutboxCounters(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics("outbox-worker", registry)

	metrics.RecordClaimed(2)
	metrics.RecordPublished()
	metrics.RecordRetry()
	metrics.RecordFailed()
	metrics.RecordPublishFailure()
	metrics.RecordLeaseRecovered(1)

	assertOutboxCounterValue(t, registry, "go_commerce_outbox_claimed_total", 2)
	assertOutboxCounterValue(t, registry, "go_commerce_outbox_published_total", 1)
	assertOutboxCounterValue(t, registry, "go_commerce_outbox_retry_total", 1)
	assertOutboxCounterValue(t, registry, "go_commerce_outbox_failed_total", 1)
	assertOutboxCounterValue(t, registry, "outbox_publish_failures_total", 1)
	assertOutboxCounterValue(t, registry, "go_commerce_outbox_lease_recovered_total", 1)
}

func assertOutboxCounterValue(t *testing.T, gatherer prometheus.Gatherer, metricName string, want float64) {
	t.Helper()

	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			if outboxMetricHasLabels(metric, map[string]string{"service": "outbox-worker"}) && metric.GetCounter().GetValue() == want {
				return
			}
		}
	}
	t.Fatalf("metric %s did not equal %.0f", metricName, want)
}

func outboxMetricHasLabels(metric *dto.Metric, labels map[string]string) bool {
	if len(metric.GetLabel()) != len(labels) {
		return false
	}
	for _, pair := range metric.GetLabel() {
		if labels[pair.GetName()] != pair.GetValue() {
			return false
		}
	}
	return true
}
