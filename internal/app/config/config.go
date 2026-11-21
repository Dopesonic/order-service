package config

import (
	"io"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Dopesonic/order-service/internal/app/config/section"
	"github.com/Dopesonic/order-service/internal/app/constant"
	msentry "github.com/Dopesonic/order-service/internal/app/monitor/sentry"
	mtracelog "github.com/Dopesonic/order-service/internal/app/monitor/tracelog"
)

type Config struct {
	Repository section.Repository
	Processor  section.Processor
	Monitor    section.Monitor
	Client     section.Client
}

var Root Config

type LoadArgs struct {
	Output          io.Writer `json:"-"`
	EnableSimpleLog bool
}

func Load(args LoadArgs) {
	zerolog.TimestampFieldName = "timestamp"
	zerolog.MessageFieldName = "msg"
	zerolog.TimeFieldFormat = time.RFC3339Nano

	if args.EnableSimpleLog {
		args.Output = zerolog.ConsoleWriter{Out: args.Output}
	}

	log.Logger = CreateLogger(zerolog.DebugLevel, args.Output)
	log.Debug().Msg("Logger initialized with debug level")

	if err := godotenv.Load(); err != nil {
		log.Warn().Err(err).Msg("Error loading .env file")
	}

	if err := envconfig.Process("APP", &Root); err != nil {
		log.Fatal().Err(err).Msg("Error loading config")
	}

	level, err := zerolog.ParseLevel(Root.Monitor.LogLevel)
	if err != nil {
		log.Warn().Str("log_level", Root.Monitor.LogLevel).Msg("Unknown log level, using debug")
		level = zerolog.DebugLevel
	}

	output := args.Output

	w, ok := msentry.Init(Root.Monitor.Sentry, msentry.Options{
		ServiceName: constant.AppName,
		Environment: Root.Monitor.Environment,
		Release:     constant.AppVersion,
	})
	if ok {
		output = zerolog.MultiLevelWriter(output, w)
	}

	log.Logger = CreateLogger(level, output)
	log.Info().Str("log_level", level.String()).
		Msg("Logger re-initialized with config level")
}

func CreateLogger(level zerolog.Level, output io.Writer) zerolog.Logger {
	return zerolog.New(output).
		Level(level).
		Hook(mtracelog.Hook{}).
		With().
		Timestamp().
		Logger()
}
