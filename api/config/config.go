// Package config implements application configuration.
package config

import (
	"log"
	"sync"

	"github.com/ilyakaznacheev/cleanenv"
)

type (
	// Config - represent top level application configuration object.
	Config struct {
		Log
		PostgreSQL
		Telegram
		Google
	}

	// Log - represents logger configuration.
	Log struct {
		Level string `env:"TS_LOG_LEVEL" env-default:"debug"`
	}

	// PostgreSQL - represents PostgreSQL database configuration.
	PostgreSQL struct {
		User     string `env:"TS_POSTGRESQL_USER"     env-default:"admin"`
		Password string `env:"TS_POSTGRESQL_PASSWORD" env-default:"postgres"`
		Host     string `env:"TS_POSTGRESQL_HOST"     env-default:"localhost"`
		Database string `env:"TS_POSTGRESQL_DATABASE" env-default:"api"`
	}

	Telegram struct {
		BotToken string `env:"TS_TELEGRAM_BOT_TOKEN" env-default:""`
	}

	// Google - represents Google API configuration.
	Google struct {
		CredentialsJSON string `env:"TS_GOOGLE_CREDENTIALS_JSON" env-default:""` // Service account JSON credentials (base64 encoded or raw JSON)
	}
)

var (
	config Config
	once   sync.Once
)

// Get returns config.
func Get() *Config {
	once.Do(func() {
		err := cleanenv.ReadEnv(&config)
		if err != nil {
			log.Fatal("failed to read env", err)
		}
	})

	return &config
}
