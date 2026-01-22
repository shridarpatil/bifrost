package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig              `yaml:"server"`
	WhatsApp  WhatsAppConfig            `yaml:"whatsapp"`
	Media     MediaConfig               `yaml:"media"`
	Webhook   WebhookConfig             `yaml:"webhook"`
	Instances map[string]InstanceConfig `yaml:"instances"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type WhatsAppConfig struct {
	StorePath string `yaml:"store_path"`
}

type MediaConfig struct {
	StorePath string `yaml:"store_path"`
	MaxSizeMB int    `yaml:"max_size_mb"`
}

type WebhookConfig struct {
	URL    string `yaml:"url"`
	Secret string `yaml:"secret"`
}

type InstanceConfig struct {
	Name  string `yaml:"name"`
	Phone string `yaml:"phone"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: 9000,
		},
		WhatsApp: WhatsAppConfig{
			StorePath: "./data/whatsapp.db",
		},
		Media: MediaConfig{
			StorePath: "./data/media",
			MaxSizeMB: 16,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) GetInstance(phoneID string) *InstanceConfig {
	if inst, ok := c.Instances[phoneID]; ok {
		return &inst
	}
	return nil
}
