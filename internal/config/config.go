package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Port       int    `json:"port"`
	ModelName  string `json:"model_name"`
	ModelAPI   string `json:"model_api"`
	AlsoSonnet bool   `json:"also_sonnet"`
}

func (c *Config) applyDefaults() {
	if c.Port == 0 {
		c.Port = 8888
	}
}

// ApplyDefaults sets zero-value fields to their defaults. Exported for use by main.
func (c *Config) ApplyDefaults() { c.applyDefaults() }

// MergeFlags applies non-zero flag values on top of base. Exported for use by main.
func MergeFlags(base, flags *Config) *Config { return merge(base, flags) }

func (c *Config) Validate() error {
	if c.ModelName == "" {
		return errors.New("--model-name is required")
	}
	if c.ModelAPI == "" {
		return errors.New("--model-api is required")
	}
	return nil
}

func (c *Config) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	f, err := os.Create(filepath.Join(dir, "config.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(c)
}

func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, "config.json")
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &cfg, nil
}

// merge applies non-zero fields from flags on top of base.
func merge(base, flags *Config) *Config {
	out := *base
	if flags.Port != 0 {
		out.Port = flags.Port
	}
	if flags.ModelName != "" {
		out.ModelName = flags.ModelName
	}
	if flags.ModelAPI != "" {
		out.ModelAPI = flags.ModelAPI
	}
	if flags.AlsoSonnet {
		out.AlsoSonnet = true
	}
	return &out
}
