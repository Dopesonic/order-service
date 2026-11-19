package monitor

type OrderCreateMetric interface {
	Success(totalPrice int64)
	Failed(err error)
}

type OrderMetrics interface {
	Create() OrderCreateMetric
}
