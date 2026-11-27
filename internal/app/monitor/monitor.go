package monitor

type OrderCreateMetric interface {
	Success(totalPrice int64)
	Failed(err error)
	PublishFailed()
}

type OrderMetrics interface {
	Create() OrderCreateMetric
}
