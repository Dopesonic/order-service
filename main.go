package main

import (
	"context"
	"log"

	"github.com/Dopesonic/order-service/internal/app/config"
	"github.com/Dopesonic/order-service/internal/app/handler/http/health"
	rprocessor "github.com/Dopesonic/order-service/internal/app/processor/http"
	rcpostgres "github.com/Dopesonic/order-service/internal/app/repository/conn/postgres"
)

func main() {
	config.Load()

	cfg := config.Root

	pgClient, err := rcpostgres.NewClient(context.Background(), cfg.Repository.Postgres)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pgClient.Close()

	log.Printf("PostgreSQL connection established")

	healthHandler := health.NewHandler()

	httpProcessor := rprocessor.NewHTTP(
		healthHandler,
		cfg.Processor.WebServer,
	)

	if err := httpProcessor.Serve(); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
