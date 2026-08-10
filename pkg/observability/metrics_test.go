package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestBusinessMetricsRecordDomainSignals(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetrics("order-service", registry)

	metrics.RecordOrderCreated(true)
	metrics.RecordOrderCreated(false)
	metrics.RecordInsufficientStock()
	metrics.RecordMQPublish("order.created", true)
	metrics.RecordMQPublish("order.created", false)
	metrics.RecordOrderCancelled(true, "user_cancelled")
	metrics.RecordOrderPaid(true)
	metrics.RecordPaymentSucceeded()
	metrics.RecordConsumerFailure("order.payment_succeeded", "payment.succeeded", true)

	assertCounterValue(t, registry, "orders_created_total", map[string]string{"service": "order-service", "result": "success"}, 1)
	assertCounterValue(t, registry, "orders_created_total", map[string]string{"service": "order-service", "result": "failure"}, 1)
	assertCounterValue(t, registry, "insufficient_stock_total", map[string]string{"service": "order-service"}, 1)
	assertCounterValue(t, registry, "go_commerce_mq_publish_total", map[string]string{"service": "order-service", "event_type": "order.created", "result": "success"}, 1)
	assertCounterValue(t, registry, "go_commerce_mq_publish_total", map[string]string{"service": "order-service", "event_type": "order.created", "result": "failure"}, 1)
	assertCounterValue(t, registry, "orders_cancelled_total", map[string]string{"service": "order-service", "result": "success", "reason": "user_cancelled"}, 1)
	assertCounterValue(t, registry, "orders_paid_total", map[string]string{"service": "order-service", "result": "success"}, 1)
	assertCounterValue(t, registry, "payments_succeeded_total", map[string]string{"service": "order-service"}, 1)
	assertCounterValue(t, registry, "consumer_failures_total", map[string]string{"service": "order-service", "consumer": "order.payment_succeeded", "event_type": "payment.succeeded", "requeue": "true"}, 1)
}

func assertCounterValue(t *testing.T, gatherer prometheus.Gatherer, metricName string, labels map[string]string, want float64) {
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
			if metricHasLabels(metric, labels) && metric.GetCounter().GetValue() == want {
				return
			}
		}
	}
	t.Fatalf("metric %s with labels %v did not equal %.0f", metricName, labels, want)
}

func metricHasLabels(metric *dto.Metric, labels map[string]string) bool {
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
