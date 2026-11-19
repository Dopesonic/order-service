package mmetric

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Dopesonic/order-service/internal/app/monitor"
)

const (
	namespace = "order"
	subsystem = "order"

	labelResult = "result"

	createResultSuccess = "success"
	createResultError   = "error"

	createTotalName = "create_total"
	createTotalHelp = "Total completed order creation attempts."

	createDurationName = "create_duration_seconds"
	createDurationHelp = "Order creation duration in seconds."

	createdAmountName = "created_amount_cents_total"
	createdAmountHelp = "Total amount of successfully created orders in cents."
)

type Order struct {
	createTotal           *prometheus.CounterVec
	createDurationSeconds *prometheus.HistogramVec
	createdAmountTotal    prometheus.Counter
}

var _ monitor.OrderMetrics = (*Order)(nil)

func NewPrometheusOrder(registerer prometheus.Registerer) (*Order, error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	total, err := counterVec(
		registerer,
		createTotalName,
		createTotalHelp,
		labelResult,
	)
	if err != nil {
		return nil, fmt.Errorf("register %s: %w", createTotalName, err)
	}

	duration, err := histogramVec(
		registerer,
		createDurationName,
		createDurationHelp,
		prometheus.DefBuckets,
		labelResult,
	)
	if err != nil {
		return nil, fmt.Errorf("register %s: %w", createDurationName, err)
	}

	amount, err := counter(
		registerer,
		createdAmountName,
		createdAmountHelp,
	)
	if err != nil {
		return nil, fmt.Errorf("register %s: %w", createdAmountName, err)
	}

	return &Order{
		createTotal:           total,
		createDurationSeconds: duration,
		createdAmountTotal:    amount,
	}, nil
}

type orderCreateMetric struct {
	parent  *Order
	started time.Time
}

func (m *Order) Create() monitor.OrderCreateMetric {
	return &orderCreateMetric{
		parent:  m,
		started: time.Now(),
	}
}

func (m *orderCreateMetric) Success(totalPrice int64) {
	m.parent.createTotal.
		WithLabelValues(createResultSuccess).
		Inc()

	m.parent.createDurationSeconds.
		WithLabelValues(createResultSuccess).
		Observe(time.Since(m.started).Seconds())

	if totalPrice > 0 {
		m.parent.createdAmountTotal.Add(float64(totalPrice))
	}
}

func (m *orderCreateMetric) Failed(_ error) {
	m.parent.createTotal.
		WithLabelValues(createResultError).
		Inc()

	m.parent.createDurationSeconds.
		WithLabelValues(createResultError).
		Observe(time.Since(m.started).Seconds())
}
