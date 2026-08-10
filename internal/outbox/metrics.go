package outbox

import "github.com/prometheus/client_golang/prometheus"

type MetricsRecorder interface {
	RecordClaimed(count int)
	RecordPublished()
	RecordRetry()
	RecordFailed()
	RecordPublishFailure()
	RecordLeaseRecovered(count int)
}

type NopMetrics struct{}

func (NopMetrics) RecordClaimed(int)        {}
func (NopMetrics) RecordPublished()         {}
func (NopMetrics) RecordRetry()             {}
func (NopMetrics) RecordFailed()            {}
func (NopMetrics) RecordPublishFailure()    {}
func (NopMetrics) RecordLeaseRecovered(int) {}

type PrometheusMetrics struct {
	claimed        prometheus.Counter
	published      prometheus.Counter
	retry          prometheus.Counter
	failed         prometheus.Counter
	publishFailure prometheus.Counter
	leaseRecovered prometheus.Counter
}

func NewPrometheusMetrics(service string, registerer prometheus.Registerer) *PrometheusMetrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	labels := prometheus.Labels{"service": service}
	metrics := &PrometheusMetrics{
		claimed: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "go_commerce_outbox_claimed_total",
			Help:        "Total number of outbox events claimed by workers.",
			ConstLabels: labels,
		}),
		published: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "go_commerce_outbox_published_total",
			Help:        "Total number of outbox events marked as published.",
			ConstLabels: labels,
		}),
		retry: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "go_commerce_outbox_retry_total",
			Help:        "Total number of outbox events scheduled for retry.",
			ConstLabels: labels,
		}),
		failed: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "go_commerce_outbox_failed_total",
			Help:        "Total number of outbox events marked as failed.",
			ConstLabels: labels,
		}),
		publishFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "outbox_publish_failures_total",
			Help:        "Total number of outbox publish failures before retry or failed handling.",
			ConstLabels: labels,
		}),
		leaseRecovered: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "go_commerce_outbox_lease_recovered_total",
			Help:        "Total number of expired outbox event leases recovered by workers.",
			ConstLabels: labels,
		}),
	}
	registerer.MustRegister(
		metrics.claimed,
		metrics.published,
		metrics.retry,
		metrics.failed,
		metrics.publishFailure,
		metrics.leaseRecovered,
	)
	return metrics
}

func (m *PrometheusMetrics) RecordClaimed(count int) {
	if m == nil || count <= 0 {
		return
	}
	m.claimed.Add(float64(count))
}

func (m *PrometheusMetrics) RecordPublished() {
	if m == nil {
		return
	}
	m.published.Inc()
}

func (m *PrometheusMetrics) RecordRetry() {
	if m == nil {
		return
	}
	m.retry.Inc()
}

func (m *PrometheusMetrics) RecordFailed() {
	if m == nil {
		return
	}
	m.failed.Inc()
}

func (m *PrometheusMetrics) RecordPublishFailure() {
	if m == nil {
		return
	}
	m.publishFailure.Inc()
}

func (m *PrometheusMetrics) RecordLeaseRecovered(count int) {
	if m == nil || count <= 0 {
		return
	}
	m.leaseRecovered.Add(float64(count))
}
