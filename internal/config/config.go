// Package config manages slingshot YAML configuration.
//
// Config file: ~/.config/slingshot/config.yml
// Example:
//
//	wechat:
//	  appid: wx1234567890
//	  secret: abcdefghijklmn
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the slingshot configuration file structure.
// It is purely dynamic: all keys are stored in Extra.
type Config struct {
	Extra map[string]any `yaml:",inline"`
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

// splitKey splits a dot-separated key path into parts.
// A backslash before a dot (\.) is treated as a literal dot,
// not a path separator. This allows keys containing dots.
func splitKey(key string) []string {
	if !strings.ContainsRune(key, '\\') {
		return strings.Split(key, ".")
	}
	var parts []string
	var buf strings.Builder
	for i := 0; i < len(key); i++ {
		if key[i] == '\\' && i+1 < len(key) && key[i+1] == '.' {
			buf.WriteByte('.')
			i++
		} else if key[i] == '.' {
			parts = append(parts, buf.String())
			buf.Reset()
		} else {
			buf.WriteByte(key[i])
		}
	}
	parts = append(parts, buf.String())
	return parts
}

// Get retrieves a config value by dot-separated key path.
// It traverses nested maps inside Extra.
// Use \. to include a literal dot in a key segment.
// Examples: "wechat.appid", "sites.fermi\.dscli\.io.dir"
func Get(cfg *Config, key string) (any, error) {
	parts := splitKey(key)
	current := any(cfg.Extra)
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unknown config key: %s", key)
		}
		current, ok = m[part]
		if !ok {
			return nil, fmt.Errorf("unknown config key: %s", key)
		}
	}
	return current, nil
}

// Set sets a config value by dot-separated key path.
// Intermediate map nodes are created as needed.
// Use \. to include a literal dot in a key segment.
func Set(cfg *Config, key string, value string) error {
	parts := splitKey(key)
	if cfg.Extra == nil {
		cfg.Extra = make(map[string]any)
	}
	current := cfg.Extra
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
		} else {
			next, ok := current[part]
			if ok {
				m, ok := next.(map[string]any)
				if !ok {
					// Overwrite with a new map
					m = make(map[string]any)
					current[part] = m
				}
				current = m
			} else {
				m := make(map[string]any)
				current[part] = m
				current = m
			}
		}
	}
	return nil
}

// Del deletes a config key by dot-separated key path.
// Use \. to include a literal dot in a key segment.
func Del(cfg *Config, key string) error {
	parts := splitKey(key)
	if len(parts) == 0 || cfg.Extra == nil {
		return nil
	}
	current := cfg.Extra
	for i, part := range parts {
		if i == len(parts)-1 {
			delete(current, part)
		} else {
			next, ok := current[part]
			if !ok {
				return nil
			}
			m, ok := next.(map[string]any)
			if !ok {
				return nil
			}
			current = m
		}
	}
	return nil
}

// AllKeys returns all config keys with their values (for listing).
// Nested maps are flattened to dot-separated keys.
func AllKeys(cfg *Config) map[string]any {
	result := make(map[string]any)
	if cfg.Extra != nil {
		flatten("", cfg.Extra, result)
	}
	return result
}

// flatten recursively flattens nested maps into dot-separated keys.
func flatten(prefix string, m map[string]any, result map[string]any) {
	for k, v := range m {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			flatten(fullKey, sub, result)
		} else {
			result[fullKey] = v
		}
	}
}

// Path returns the default config file path.
func Path() string {
	return defaultConfigPath()
}
