// Package config manages slingshot YAML configuration.
//
// Config file: ~/.config/slingshot/config.yml
// Example:
//
//	wechat:
//	  appid: wx1234567890
//	  secret: abcdefghijklmn
//	default_remote: local
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the slingshot configuration file structure.
type Config struct {
	Wechat        WechatConfig    `yaml:"wechat"`
	DefaultRemote string          `yaml:"default_remote"`
	Extra         map[string]any  `yaml:",inline"`
}

// WechatConfig holds WeChat API credentials.
type WechatConfig struct {
	AppID  string `yaml:"appid"`
	Secret string `yaml:"secret"`
}

// defaultConfigPath returns the default config file path.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "slingshot", "config.yml")
}

// Load reads the configuration from the default path.
// If the file doesn't exist, returns an empty config (no error).
func Load() (*Config, string, error) {
	path := defaultConfigPath()
	if path == "" {
		return nil, "", fmt.Errorf("cannot determine home directory")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config, no error
			return &Config{}, path, nil
		}
		return nil, path, fmt.Errorf("reading config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, path, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, path, nil
}

// Save writes the configuration to the given path.
func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// Get retrieves a config value by dot-separated key path.
// Examples: "wechat.appid", "default_remote", "extra.key"
func Get(cfg *Config, key string) (any, error) {
	switch key {
	case "wechat.appid":
		return cfg.Wechat.AppID, nil
	case "wechat.secret":
		return cfg.Wechat.Secret, nil
	case "default_remote":
		return cfg.DefaultRemote, nil
	default:
		if cfg.Extra != nil {
			if v, ok := cfg.Extra[key]; ok {
				return v, nil
			}
		}
		return nil, fmt.Errorf("unknown config key: %s", key)
	}
}

// Set sets a config value by dot-separated key path.
func Set(cfg *Config, key string, value string) error {
	switch key {
	case "wechat.appid":
		cfg.Wechat.AppID = value
		return nil
	case "wechat.secret":
		cfg.Wechat.Secret = value
		return nil
	case "default_remote":
		cfg.DefaultRemote = value
		return nil
	default:
		if cfg.Extra == nil {
			cfg.Extra = make(map[string]any)
		}
		cfg.Extra[key] = value
		return nil
	}
}

// Del deletes a config key.
func Del(cfg *Config, key string) error {
	switch key {
	case "wechat.appid":
		cfg.Wechat.AppID = ""
		return nil
	case "wechat.secret":
		cfg.Wechat.Secret = ""
		return nil
	case "default_remote":
		cfg.DefaultRemote = ""
		return nil
	default:
		if cfg.Extra != nil {
			delete(cfg.Extra, key)
		}
		return nil
	}
}

// AllKeys returns all config keys with their values (for listing).
func AllKeys(cfg *Config) map[string]any {
	result := make(map[string]any)
	result["wechat.appid"] = cfg.Wechat.AppID
	result["wechat.secret"] = cfg.Wechat.Secret
	result["default_remote"] = cfg.DefaultRemote
	for k, v := range cfg.Extra {
		result[k] = v
	}
	return result
}

// Path returns the default config file path.
func Path() string {
	return defaultConfigPath()
}
