package config

import (
	"os"
	"strconv"
)

const (
	DefaultMaxContextSize = 200000 // 200KB in bytes
)

type Config struct {
	MaxContextSize int
	AnthropicKey   string
}

func New() *Config {
	cfg := &Config{
		MaxContextSize: DefaultMaxContextSize,
		AnthropicKey:   os.Getenv("ANTHROPIC_API_KEY"),
	}

	if maxSize := os.Getenv("REPOCONTEXT_MAX_SIZE"); maxSize != "" {
		if size, err := strconv.Atoi(maxSize); err == nil {
			cfg.MaxContextSize = size
		}
	}

	return cfg
}
