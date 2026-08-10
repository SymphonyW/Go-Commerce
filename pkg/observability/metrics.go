package observability

import (
	"strconv"
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
	ordersCancelled         *prometheus.CounterVec
	ordersPaid              *prometheus.CounterVec
	paymentResults          *prometheus.CounterVec
	paymentsSucceeded       prometheus.Counter
	insufficientStock       prometheus.Counter
	mqPublish               *prometheus.CounterVec
	consumerFailures        *prometheus.CounterVec
	registeredOutboxPending bool
	registeredOutboxOldest  bool
}

func NewMetrics(service string, registerer prometheus.Registerer) *Metrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	labels := prometheus.Labels{"service": service}
	metrics := &Metrics{
		service: service,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "http_requests_total",
			Help:        "Total number of HTTP requests.",
			ConstLabels: labels,
		}, []string{"method", "path", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "http_request_duration_seconds",
			Help:        "HTTP request duration in seconds.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"method", "path"}),
		grpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "grpc_server_requests_total",
			Help:        "Total number of gRPC requests.",
			ConstLabels: labels,
		}, []string{"method", "code"}),
		grpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "grpc_server_duration_seconds",
			Help:        "gRPC request duration in seconds.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"method"}),
		orderCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "orders_created_total",
			Help:        "Number of order creation attempts grouped by result.",
			ConstLabels: labels,
		}, []string{"result"}),
		ordersCancelled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "orders_cancelled_total",
			Help:        "Number of order cancellation attempts grouped by result and reason.",
			ConstLabels: labels,
		}, []string{"result", "reason"}),
		ordersPaid: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "orders_paid_total",
			Help:        "Number of order paid transitions grouped by result.",
			ConstLabels: labels,
		}, []string{"result"}),
		paymentResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "go_commerce_payment_total",
			Help:        "Number of payment actions grouped by result.",
			ConstLabels: labels,
		}, []string{"result"}),
		paymentsSucceeded: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "payments_succeeded_total",
			Help:        "Number of successful payment transitions.",
			ConstLabels: labels,
		}),
		insufficientStock: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "insufficient_stock_total",
			Help:        "Number of insufficient stock rejections.",
			ConstLabels: labels,
		}),
		mqPublish: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "go_commerce_mq_publish_total",
			Help:        "Number of MQ publish attempts grouped by event type and result.",
			ConstLabels: labels,
		}, []string{"event_type", "result"}),
		consumerFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "consumer_failures_total",
			Help:        "Number of MQ consumer failures grouped by consumer, event type, and retry behavior.",
			ConstLabels: labels,
		}, []string{"consumer", "event_type", "requeue"}),
	}

	registerer.MustRegister(
		metrics.httpRequests,
		metrics.httpDuration,
		metrics.grpcRequests,
		metrics.grpcDuration,
		metrics.orderCreated,
		metrics.ordersCancelled,
		metrics.ordersPaid,
		metrics.paymentResults,
		metrics.paymentsSucceeded,
		metrics.insufficientStock,
		metrics.mqPublish,
		metrics.consumerFailures,
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
	if success {
		m.paymentsSucceeded.Inc()
	}
}

func (m *Metrics) RecordPaymentSucceeded() {
	if m == nil {
		return
	}
	m.RecordPaymentResult(true)
}

func (m *Metrics) RecordOrderCancelled(success bool, reason string) {
	if m == nil {
		return
	}
	if reason == "" {
		reason = "unspecified"
	}
	m.ordersCancelled.WithLabelValues(resultLabel(success), reason).Inc()
}

func (m *Metrics) RecordOrderPaid(success bool) {
	if m == nil {
		return
	}
	m.ordersPaid.WithLabelValues(resultLabel(success)).Inc()
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

func (m *Metrics) RecordConsumerFailure(consumer, eventType string, requeue bool) {
	if m == nil {
		return
	}
	if consumer == "" {
		consumer = "unknown"
	}
	if eventType == "" {
		eventType = "unknown"
	}
	m.consumerFailures.WithLabelValues(consumer, eventType, strconv.FormatBool(requeue)).Inc()
}

func (m *Metrics) RegisterOutboxPendingGauge(registerer prometheus.Registerer, count func() float64) {
	if m == nil || m.registeredOutboxPending {
		return
	}
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name:        "outbox_pending_events",
		Help:        "Current number of pending outbox events.",
		ConstLabels: prometheus.Labels{"service": m.service},
	}, count))
	m.registeredOutboxPending = true
}

func (m *Metrics) RegisterOutboxOldestPendingGauge(registerer prometheus.Registerer, ageSeconds func() float64) {
	if m == nil || m.registeredOutboxOldest {
		return
	}
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name:        "outbox_oldest_pending_seconds",
		Help:        "Age in seconds of the oldest pending outbox event.",
		ConstLabels: prometheus.Labels{"service": m.service},
	}, ageSeconds))
	m.registeredOutboxOldest = true
}

func resultLabel(success bool) string {
	if success {
		return "success"
	}
	return "failure"
}
