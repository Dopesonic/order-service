package sorder

import (
	"context"
	"time"

	"github.com/gofrs/uuid"

	"github.com/Dopesonic/order-service/internal/app/entity"
	"github.com/Dopesonic/order-service/internal/app/repository"
	"github.com/Dopesonic/order-service/internal/app/service"
)

type srv struct {
	repoOrder repository.Order
}

func NewService(repoOrder repository.Order) service.Order {
	return &srv{
		repoOrder: repoOrder,
	}
}

func (s *srv) Create(ctx context.Context, req entity.RequestOrderCreate) (entity.Order, error) {
	start := time.Now()
	orderGUID := uuid.Must(uuid.NewV4())

	items := make([]entity.OrderItem, 0, len(req.Items))
	var totalPrice int64

	for _, item := range req.Items {
		totalPrice += item.Quantity * item.UnitPrice

		items = append(items, entity.OrderItem{
			GUID:        uuid.Must(uuid.NewV4()),
			OrderGUID:   orderGUID,
			ProductGUID: item.ProductGUID,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			CreatedAt:   start,
			UpdatedAt:   start,
		})
	}

	order := entity.Order{
		GUID:       orderGUID,
		UserGUID:   req.UserGUID,
		TotalPrice: totalPrice,
		Currency:   req.Currency,
		Status:     "pending",
		CreatedAt:  start,
		UpdatedAt:  start,
		Items:      items,
	}

	if err := s.repoOrder.Create(ctx, order); err != nil {
		return entity.Order{}, err
	}

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
