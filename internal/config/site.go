// Package config manages slingshot YAML configuration.
//
// Site config helpers.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Site represents a deployment site in the configuration.
type Site struct {
	Dir   string `yaml:"dir"`
	Rsync string `yaml:"rsync"`
}

// GetSites returns all configured sites.
// The Dir field is expanded: "~" prefix is replaced with the user's home directory.
func GetSites(cfg *Config) map[string]Site {
	sites := make(map[string]Site)
	raw, err := Get(cfg, "sites")
	if err != nil {
		return sites
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return sites
	}
	home, _ := os.UserHomeDir()
	for name, val := range rawMap {
		siteMap, ok := val.(map[string]any)
		if !ok {
			continue
		}
		site := Site{}
		if dir, ok := siteMap["dir"].(string); ok {
			// Expand ~ to home directory
			if strings.HasPrefix(dir, "~/") && home != "" {
				dir = home + dir[1:]
			}
			site.Dir = dir
		}
		if rsync, ok := siteMap["rsync"].(string); ok {
			site.Rsync = rsync
		}
		sites[name] = site
	}
	return sites
}

// GetSite returns a single site configuration by name.
// The Dir field is expanded: "~" prefix is replaced with the user's home directory.
func GetSite(cfg *Config, name string) (Site, bool) {
	sites := GetSites(cfg)
	site, ok := sites[name]
	return site, ok
}

// EscapeName escapes dots in a site name so it can be used as a single
// key segment in Set/Del paths.
func EscapeName(name string) string {
	return strings.ReplaceAll(name, ".", "\\.")
}

// AddSite adds or updates a site in the configuration.
func AddSite(cfg *Config, name string, site Site) error {
	escaped := EscapeName(name)
	if err := Set(cfg, "sites."+escaped+".dir", site.Dir); err != nil {
		return fmt.Errorf("setting site dir: %w", err)
	}
	if err := Set(cfg, "sites."+escaped+".rsync", site.Rsync); err != nil {
		return fmt.Errorf("setting site rsync: %w", err)
	}
	return nil
}

// RemoveSite removes a site from the configuration.
func RemoveSite(cfg *Config, name string) error {
	escaped := EscapeName(name)
	if err := Del(cfg, "sites."+escaped); err != nil {
		return fmt.Errorf("removing site: %w", err)
	}
	// Clean up empty sites map
	if raw, ok := cfg.Extra["sites"]; ok {
		if m, ok := raw.(map[string]any); ok && len(m) == 0 {
			delete(cfg.Extra, "sites")
		}
	}
	return nil
}
