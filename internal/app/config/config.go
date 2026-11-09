package config

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"

	"github.com/Dopesonic/order-service/internal/app/config/section"
)

type Config struct {
	Repository section.Repository
	Processor  section.Processor
	Monitor    section.Monitor
}

var Root Config

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file")
	}

	if err := envconfig.Process("APP", &Root); err != nil {
		log.Fatalf("Error loading config: %s", err)
	}
}
