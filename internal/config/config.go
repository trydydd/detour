package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
	if err := validateModelAPI(c.ModelAPI); err != nil {
		return err
	}
	return nil
}

// validateModelAPI checks that the ModelAPI URL has a valid scheme and no path/query/fragment.
func validateModelAPI(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid model-api URL: %v", err)
	}

	// Only http and https schemes are allowed
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("model-api must use http or https scheme (got %q)", parsed.Scheme)
	}

	// Reject URLs with path, query, or fragment components
	if parsed.Path != "" {
		return fmt.Errorf("model-api must not include a path (got %q)", parsed.Path)
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("model-api must not include query parameters (got %q)", parsed.RawQuery)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("model-api must not include a fragment (got %q)", parsed.Fragment)
	}

	// Require a host
	if parsed.Host == "" {
		return errors.New("model-api must include a host")
	}

	return nil
}

// CleartextUpstreamWarning returns a warning string if modelAPI uses http://
// with a non-loopback host, otherwise returns an empty string.
func CleartextUpstreamWarning(modelAPI string) string {
	parsed, err := url.Parse(modelAPI)
	if err != nil || parsed.Scheme != "http" {
		return ""
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return ""
	}
	return "detour: WARNING: --model-api uses http:// to a non-loopback host — " +
		"your prompts and responses will be sent unencrypted over the network.\n" +
		"detour: WARNING: Use an SSH tunnel (ssh -L) or TLS termination to protect this traffic."
}

func (c *Config) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "config.json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(c)
}

func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, "config.json")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return nil, fmt.Errorf("config file %s has insecure permissions %04o (want 0600)", path, perm)
	}
	f, err := os.Open(path)
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
