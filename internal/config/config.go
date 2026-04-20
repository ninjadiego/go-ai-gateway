package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v10"
)

type Config struct {
	Port     int    `env:"PORT" envDefault:"8080"`
	Env      string `env:"ENV" envDefault:"development"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	DB        DBConfig
	Anthropic AnthropicConfig

	AdminToken      string        `env:"ADMIN_TOKEN,required"`
	UpstreamTimeout time.Duration `env:"UPSTREAM_TIMEOUT" envDefault:"60s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

type DBConfig struct {
	Host         string `env:"DB_HOST" envDefault:"localhost"`
	Port         int    `env:"DB_PORT" envDefault:"3306"`
	User         string `env:"DB_USER,required"`
	Password     string `env:"DB_PASSWORD,required"`
	Name         string `env:"DB_NAME,required"`
	MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns int    `env:"DB_MAX_IDLE_CONNS" envDefault:"10"`
}

func (d DBConfig) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=UTC",
		d.User, d.Password, d.Host, d.Port, d.Name,
	)
}

type AnthropicConfig struct {
	APIKey       string `env:"ANTHROPIC_API_KEY,required"`
	BaseURL      string `env:"ANTHROPIC_BASE_URL" envDefault:"https://api.anthropic.com"`
	DefaultModel string `env:"ANTHROPIC_DEFAULT_MODEL" envDefault:"claude-sonnet-4-6"`
}

// Load parses environment variables into a Config. Fails fast on missing required vars.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
