package builder

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"

	ccatalog "github.com/Dopesonic/order-service/internal/app/client/catalog"
	cgrpc "github.com/Dopesonic/order-service/internal/app/client/catalog/grpc"
	"github.com/Dopesonic/order-service/internal/app/config"
	"github.com/Dopesonic/order-service/internal/app/constant"
	"github.com/Dopesonic/order-service/internal/app/entity"
	rhandler "github.com/Dopesonic/order-service/internal/app/handler/http"
	rhealth "github.com/Dopesonic/order-service/internal/app/handler/http/health"
	horder "github.com/Dopesonic/order-service/internal/app/handler/http/order"
	"github.com/Dopesonic/order-service/internal/app/monitor/metric"
	"github.com/Dopesonic/order-service/internal/app/processor"
	rprocessor "github.com/Dopesonic/order-service/internal/app/processor/http"
	mmonitor "github.com/Dopesonic/order-service/internal/app/processor/monitor"
	"github.com/Dopesonic/order-service/internal/app/repository"
	rcpostgres "github.com/Dopesonic/order-service/internal/app/repository/conn/postgres"
	porder "github.com/Dopesonic/order-service/internal/app/repository/order"
	"github.com/Dopesonic/order-service/internal/app/service"
	sorder "github.com/Dopesonic/order-service/internal/app/service/order"
	"github.com/Dopesonic/order-service/internal/pkg/broker"
	"github.com/Dopesonic/order-service/internal/pkg/broker/codec"
)

type Builder struct {
	cCtx            *cli.Context
	ctx             context.Context
	wg              sync.WaitGroup
	err             error
	cfg             config.Config
	otelServiceName string

	chErrors chan error

	connPostgres *rcpostgres.Client

	orderRepo repository.Order

	orderService service.Order

	healthHandler   rhandler.Health
	orderHandler    rhandler.Order
	catalogV1Client ccatalog.Client
	catalogGrpcConn *grpc.ClientConn
	kafkaClient     *broker.KafkaClient
	busOrderCreated broker.Bus[entity.EventOrderCreated]
	processors      []processor.Processor
}

func NewBuilder(cCtx *cli.Context) *Builder {
	b := Builder{
		cCtx:     cCtx,
		chErrors: make(chan error, 4096),
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.ctx = ctx

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go b.waitForSignal(sig, cancel)
	go b.printErrors()

	b.healthHandler = rhealth.NewHandler()

	return &b
}

func (b *Builder) BuildConfig() {
	b.exec(func(b *Builder) {
		b.buildConfig()
	})
}

func (b *Builder) Run() {
	if b.ctx.Err() != nil {
		log.Info().Msg("Shutdown during initialization")
		return
	}

	if b.err != nil {
		log.Fatal().Err(b.err).Msg("Failed to initialize application")
	}
	log.Info().Msg("Application initialized")
	defer log.Info().Msg("Application completed")

	for _, proc := range b.processors {
		proc.StartAsync(b.ctx, &b.wg)
	}

	b.wg.Wait()
}

////////////////////////////////////////////////////////////////////////////
///// REPOSITORY CONNECTIONS ///////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

func (b *Builder) BuildRepoConnPostgres() {
	b.exec(func(b *Builder) {
		client, err := rcpostgres.NewClient(b.ctx, b.cfg.Repository.Postgres)
		if err != nil {
			b.err = err
			return
		}
		b.connPostgres = client
	})
}

func (b *Builder) BuildRepoOrder() {
	b.exec(func(b *Builder) {
		b.orderRepo = porder.NewRepo(b.connPostgres)
	}, b.connPostgres)
}

////////////////////////////////////////////////////////////////////////////////
///// SERVICES /////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

func (b *Builder) BuildServiceOrder() {
	b.exec(func(b *Builder) {
		orderMetrics, err := mmetric.NewPrometheusOrder(
			prometheus.DefaultRegisterer,
		)
		if err != nil {
			b.err = fmt.Errorf("init order metrics: %w", err)
			return
		}
		b.orderService = sorder.NewService(b.orderRepo, b.catalogV1Client, orderMetrics, b.busOrderCreated)
	}, b.orderRepo, b.catalogV1Client, b.busOrderCreated)
}

////////////////////////////////////////////////////////////////////////////////
///// HANDLERS /////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

func (b *Builder) BuildHandlerOrder() {
	b.exec(func(b *Builder) {
		b.orderHandler = horder.NewHandler(b.orderService)
	}, b.orderService)
}

////////////////////////////////////////////////////////////////////////////
///// PROCESSORS ///////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

func (b *Builder) BuildProcHttp() {
	b.exec(func(b *Builder) {
		p := rprocessor.NewHTTP(
			b.otelServiceName,
			b.healthHandler,
			b.orderHandler,
			b.cfg.Processor.WebServer,
		)
		b.processors = append(b.processors, p)
	}, b.healthHandler, b.orderHandler)
}

func (b *Builder) BuildClientGrpcCatalogV1() {
	b.exec(func(b *Builder) {
		client, conn, err := cgrpc.NewClient(b.cfg.Client.Catalog.GrpcAddress)
		if err != nil {
			b.err = fmt.Errorf("failed create catalog gRPC client: %w", err)
			return
		}

		pingCtx, cancel := context.WithTimeout(b.ctx, 2*time.Second)
		defer cancel()

		if err := client.Ping(pingCtx); err != nil {
			if closeErr := conn.Close(); closeErr != nil {
				log.Error().
					Err(closeErr).
					Msg("Failed to close catalog gRPC connection")
			}

			b.err = fmt.Errorf("failed ping catalog service: %w", err)
			return
		}

		b.catalogV1Client = client
		b.catalogGrpcConn = conn

		processor.WatchForShutdown(
			b.ctx,
			&b.wg,
			b.catalogGrpcConn,
		)

		log.Info().
			Str("address", b.cfg.Client.Catalog.GrpcAddress).
			Msg("Catalog gRPC client initialized")
	})
}

func (b *Builder) BuildMonitorOpenTelemetry() {
	cfg := b.cfg.Monitor.OpenTelemetry
	if !cfg.Enabled {
		log.Warn().Msg("OpenTelemetry is disabled by config")
		return
	}

	b.exec(func(b *Builder) {
		proc, err := mmonitor.NewOpenTelemetryController(b.ctx, b.cfg.Monitor.Environment, cfg)
		if err != nil {
			b.err = fmt.Errorf("init OpenTelemetry: %w", err)
			return
		}
		b.processors = append(b.processors, proc)
		b.otelServiceName = constant.AppName
	})
}

////////////////////////////////////////////////////////////////////////////
///// BROKER ///////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

func (b *Builder) BuildBrokerKafka() {
	b.exec(func(b *Builder) {
		cfg := b.cfg.Broker.Kafka

		client, err := broker.NewKafkaClient(broker.KafkaConfig{
			Addresses:     cfg.Addresses,
			ConsumerGroup: cfg.ConsumerGroup,
			ClientID:      cfg.ClientID,
		})
		if err != nil {
			b.err = fmt.Errorf("init broker kafka client: %w", err)
		}

		b.kafkaClient = client

		topic := cfg.ModelOrder.Created.Topic

		group := broker.Coalesce(cfg.ModelOrder.Created.ConsumerGroup, cfg.ConsumerGroup)

		bus, err := broker.NewBus[entity.EventOrderCreated](
			client,
			codec.NewCodecJson[entity.EventOrderCreated](),
			topic,
			group,
		)
		if err != nil {
			_ = client.Close()
			b.err = fmt.Errorf("init broker kafka bus: %w", err)
			return
		}

		b.busOrderCreated = bus

		shutdown := processor.ProcessorFunc(func(ctx context.Context, wg *sync.WaitGroup) {
			processor.WatchForShutdown(ctx, wg, processor.CloserFunc(client.Close))
		})
		b.processors = append(b.processors, shutdown)
	})
}

////////////////////////////////////////////////////////////////////////////
///// PRIVATE //////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

func (b *Builder) waitForSignal(sig chan os.Signal, cancelFunc func()) {
	receivedSignal := <-sig

	log.Info().Str("signal", receivedSignal.String()).Msg("Shutdown is requested")

	cancelFunc()
}

func (b *Builder) printErrors() {
	for err := range b.chErrors {
		log.Error().Err(err).Msg("Got new error")
	}
}

func (b *Builder) buildConfig() {
	args := config.LoadArgs{Output: b.cCtx.App.Writer, EnableSimpleLog: b.cCtx.Bool("no-json")}
	config.Load(args)
	b.cfg = config.Root
}

func (b *Builder) exec(cb func(b *Builder), requiredArgs ...any) {
	if b.ctx.Err() != nil {
		return
	}

	if b.err != nil {
		return
	}

	for i, requiredArg := range requiredArgs {
		rv := reflect.ValueOf(requiredArg)
		if !rv.IsValid() {
			b.err = fmt.Errorf("BUG: required argument #%d is nil (check dependencies)", i)
			return
		}
		if rv.Type().Kind() == reflect.Struct || !rv.IsZero() {
			continue
		}
		b.err = fmt.Errorf("BUG: required %s, but empty", rv.Type().String())
		return
	}

	cb(b)
}
