package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	service string

	httpRequests            *prometheus.CounterVec
	httpDuration            *prometheus.HistogramVec
	grpcRequests            *prometheus.CounterVec
	grpcDuration            *prometheus.HistogramVec
	orderCreated            *prometheus.CounterVec
	paymentResults          *prometheus.CounterVec
	insufficientStock       prometheus.Counter
	mqPublish               *prometheus.CounterVec
	registeredOutboxPending bool
}

func NewMetrics(service string, registerer prometheus.Registerer) *Metrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	labels := prometheus.Labels{"service": service}
	metrics := &Metrics{
		service: service,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "go_commerce_http_requests_total",
			Help:        "Total number of HTTP requests.",
			ConstLabels: labels,
		}, []string{"method", "path", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "go_commerce_http_request_duration_seconds",
			Help:        "HTTP request duration in seconds.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"method", "path"}),
		grpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "go_commerce_grpc_requests_total",
			Help:        "Total number of gRPC requests.",
			ConstLabels: labels,
		}, []string{"method", "code"}),
		grpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "go_commerce_grpc_request_duration_seconds",
			Help:        "gRPC request duration in seconds.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"method"}),
		orderCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "go_commerce_order_created_total",
			Help:        "Number of order creation attempts grouped by result.",
			ConstLabels: labels,
		}, []string{"result"}),
		paymentResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "go_commerce_payment_total",
			Help:        "Number of payment actions grouped by result.",
			ConstLabels: labels,
		}, []string{"result"}),
		insufficientStock: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "go_commerce_inventory_insufficient_total",
			Help:        "Number of insufficient stock rejections.",
			ConstLabels: labels,
		}),
		mqPublish: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "go_commerce_mq_publish_total",
			Help:        "Number of MQ publish attempts grouped by event type and result.",
			ConstLabels: labels,
		}, []string{"event_type", "result"}),
	}

	registerer.MustRegister(
		metrics.httpRequests,
		metrics.httpDuration,
		metrics.grpcRequests,
		metrics.grpcDuration,
		metrics.orderCreated,
		metrics.paymentResults,
		metrics.insufficientStock,
		metrics.mqPublish,
	)
	return metrics
}

func (m *Metrics) ObserveHTTP(method, path, status string, duration time.Duration) {
	if m == nil {
		return
	}
	m.httpRequests.WithLabelValues(method, path, status).Inc()
	m.httpDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func (m *Metrics) ObserveGRPC(method, code string, duration time.Duration) {
	if m == nil {
		return
	}
	m.grpcRequests.WithLabelValues(method, code).Inc()
	m.grpcDuration.WithLabelValues(method).Observe(duration.Seconds())
}

func (m *Metrics) RecordOrderCreated(success bool) {
	if m == nil {
		return
	}
	m.orderCreated.WithLabelValues(resultLabel(success)).Inc()
}

func (m *Metrics) RecordPaymentResult(success bool) {
	if m == nil {
		return
	}
	m.paymentResults.WithLabelValues(resultLabel(success)).Inc()
}

func (m *Metrics) RecordInsufficientStock() {
	if m == nil {
		return
	}
	m.insufficientStock.Inc()
}

func (m *Metrics) RecordMQPublish(eventType string, success bool) {
	if m == nil {
		return
	}
	m.mqPublish.WithLabelValues(eventType, resultLabel(success)).Inc()
}

func (m *Metrics) RegisterOutboxPendingGauge(registerer prometheus.Registerer, count func() float64) {
	if m == nil || m.registeredOutboxPending {
		return
	}
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name:        "go_commerce_outbox_pending",
		Help:        "Current number of pending outbox events.",
		ConstLabels: prometheus.Labels{"service": m.service},
	}, count))
	m.registeredOutboxPending = true
}

func resultLabel(success bool) string {
	if success {
		return "success"
	}
	return "failure"
}
