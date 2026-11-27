package sorder

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/rs/zerolog/log"

	ccatalog "github.com/Dopesonic/order-service/internal/app/client/catalog"
	"github.com/Dopesonic/order-service/internal/app/entity"
	"github.com/Dopesonic/order-service/internal/app/monitor"
	"github.com/Dopesonic/order-service/internal/app/repository"
	"github.com/Dopesonic/order-service/internal/app/service"
	"github.com/Dopesonic/order-service/internal/pkg/broker"
	catalogv1 "github.com/Dopesonic/order-service/internal/pkg/grpc/gen/catalog/v1"
)

type srv struct {
	repoOrder       repository.Order
	catalogGrpc     ccatalog.Client
	metrics         monitor.OrderMetrics
	busOrderCreated broker.Bus[entity.EventOrderCreated]
}

//nolint:unused // Reserved for service unit tests.
type noopOrderMetrics struct{}

//nolint:unused // Reserved for service unit tests.
type noopOrderCreateMetric struct{}

//nolint:unused // Reserved for service unit tests.
func (noopOrderMetrics) Create() monitor.OrderCreateMetric {
	return noopOrderCreateMetric{}
}

//nolint:unused // Reserved for service unit tests.
func (noopOrderCreateMetric) Success(int64) {}

//nolint:unused // Reserved for service unit tests.
func (noopOrderCreateMetric) Failed(error) {}

//nolint:unused // заготовка no-op метрик (Null Object) — в order пока не используется в тестах
func (noopOrderCreateMetric) PublishFailed() {}

func NewService(
	repoOrder repository.Order,
	catalogGrpc ccatalog.Client,
	metrics monitor.OrderMetrics,
	busOrderCreated broker.Bus[entity.EventOrderCreated],
) service.Order {
	return &srv{
		repoOrder:       repoOrder,
		catalogGrpc:     catalogGrpc,
		metrics:         metrics,
		busOrderCreated: busOrderCreated,
	}
}

func (s *srv) Create(ctx context.Context, req entity.RequestOrderCreate) (entity.Order, error) {
	createMetric := s.metrics.Create()

	productGUIDs := make([]string, 0, len(req.Items))

	for _, item := range req.Items {
		productGUIDs = append(productGUIDs, item.ProductGUID.String())
	}

	productsResp, err := s.catalogGrpc.GetProducts(
		ctx,
		&catalogv1.GetProductsRequest{
			Guids: productGUIDs,
		},
	)
	if err != nil {
		createMetric.Failed(err)
		return entity.Order{}, err
	}

	if len(productsResp.GetMissingGuids()) > 0 {
		createMetric.Failed(entity.ErrIncorrectParameters)
		return entity.Order{}, entity.ErrIncorrectParameters
	}

	priceByGUID := make(map[string]int64, len(productsResp.GetProducts()))

	for _, product := range productsResp.GetProducts() {
		priceByGUID[product.GetGuid()] = product.GetPrice()
	}

	now := time.Now()
	orderGUID := uuid.Must(uuid.NewV4())

	items := make([]entity.OrderItem, 0, len(req.Items))
	var totalPrice int64

	for _, item := range req.Items {
		unitPrice, found := priceByGUID[item.ProductGUID.String()]
		if !found {
			createMetric.Failed(entity.ErrIncorrectParameters)
			return entity.Order{}, entity.ErrIncorrectParameters
		}

		totalPrice += item.Quantity * unitPrice

		items = append(items, entity.OrderItem{
			GUID:        uuid.Must(uuid.NewV4()),
			OrderGUID:   orderGUID,
			ProductGUID: item.ProductGUID,
			Quantity:    item.Quantity,
			UnitPrice:   unitPrice,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	order := entity.Order{
		GUID:       orderGUID,
		UserGUID:   req.UserGUID,
		TotalPrice: totalPrice,
		Currency:   req.Currency,
		Status:     "pending",
		CreatedAt:  now,
		UpdatedAt:  now,
		Items:      items,
	}

	if err := s.repoOrder.Create(ctx, order); err != nil {
		createMetric.Failed(err)
		return entity.Order{}, err
	}

	eventItem := make([]entity.EventOrderCreatedItem, len(order.Items))

	for i, item := range order.Items {
		eventItem[i] = entity.EventOrderCreatedItem{
			ProductGUID: item.ProductGUID.String(),
			Quantity:    int(item.Quantity),
			UnitPrice:   item.UnitPrice,
		}
	}

	ev := entity.EventOrderCreated{
		OrderGUID:  order.GUID.String(),
		Currency:   order.Currency,
		TotalPrice: order.TotalPrice,
		Items:      eventItem,
		CreatedAt:  order.CreatedAt.UTC().Format(time.RFC3339),
	}

	if order.UserGUID != nil {
		userGuid := order.UserGUID.String()
		ev.UserGUID = &userGuid
	}

	if err := s.busOrderCreated.Send(ctx, &ev, entity.BrokerHeaderOrderCreatedType(),
		entity.BrokerHeaderOrderCreatedEventID()); err != nil {
		log.Error().
			Err(err).
			EmbedObject(ev).
			Msg("failed to publish order.created")

		createMetric.PublishFailed()
	}

	createMetric.Success(order.TotalPrice)
	return order, nil
}

func (s *srv) GetByGUID(ctx context.Context, guid uuid.UUID) (entity.Order, error) {
	return s.repoOrder.GetByGUID(ctx, guid)
}

func (s *srv) Update(ctx context.Context, guid uuid.UUID, req entity.RequestOrderUpdate) (entity.Order, error) {
	var updatedOrder entity.Order

	err := s.repoOrder.InsideTx(ctx, func(txCtx context.Context) error {
		order, err := s.repoOrder.GetByGUID(txCtx, guid)
		if err != nil {
			return err
		}

		order.Status = req.Status
		order.UpdatedAt = time.Now()

		if err := s.repoOrder.Update(txCtx, order); err != nil {
			return err
		}
		updatedOrder = order

		return nil
	})
	if err != nil {
		return entity.Order{}, err
	}

	return updatedOrder, nil
}

func (s *srv) Delete(ctx context.Context, guid uuid.UUID) error {
	return s.repoOrder.InsideTx(ctx, func(txCtx context.Context) error {
		if _, err := s.repoOrder.GetByGUID(txCtx, guid); err != nil {
			return err
		}

		return s.repoOrder.Delete(txCtx, guid)
	})
}

func (s *srv) List(ctx context.Context, req entity.RequestOrderList) ([]entity.Order, error) {
	return s.repoOrder.List(ctx, req.Status, req.UserGUID)
}
