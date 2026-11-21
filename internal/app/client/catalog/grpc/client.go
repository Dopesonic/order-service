package cgrpc

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	ccatalog "github.com/Dopesonic/order-service/internal/app/client/catalog"
	"github.com/Dopesonic/order-service/internal/app/entity"
	catalogv1 "github.com/Dopesonic/order-service/internal/pkg/grpc/gen/catalog/v1"
)

type client struct {
	raw catalogv1.CatalogServiceClient
}

var _ ccatalog.Client = (*client)(nil)

var grpcCodeToAppError = map[codes.Code]error{
	codes.NotFound:        entity.ErrNotFound,
	codes.InvalidArgument: entity.ErrIncorrectParameters,
	codes.AlreadyExists:   entity.ErrAlreadyExists,
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}

	grpcStatus, ok := status.FromError(err)
	if !ok {
		return entity.ErrInternal
	}
	if appErr, found := grpcCodeToAppError[grpcStatus.Code()]; found {
		return appErr
	}

	return entity.ErrInternal
}

func NewClient(addr string) (ccatalog.Client, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, nil, err
	}

	raw := catalogv1.NewCatalogServiceClient(conn)

	return &client{raw: raw}, conn, nil
}

func (c *client) Ping(ctx context.Context) error {
	_, err := c.raw.GetProducts(ctx, &catalogv1.GetProductsRequest{})
	return err
}

func (c *client) GetProducts(
	ctx context.Context,
	req *catalogv1.GetProductsRequest,
) (*catalogv1.GetProductsResponse, error) {
	resp, err := c.raw.GetProducts(ctx, req)
	if err != nil {
		return nil, normalizeError(err)
	}

	return resp, nil
}

func (c *client) GetProduct(
	ctx context.Context,
	req *catalogv1.GetProductRequest,
) (*catalogv1.GetProductResponse, error) {
	resp, err := c.raw.GetProduct(ctx, req)
	if err != nil {
		return nil, normalizeError(err)
	}

	return resp, nil
}
