// Package config manages slingshot YAML configuration.
//
// Site config helpers.
package config

import "fmt"

// Site represents a deployment site in the configuration.
type Site struct {
	Dir   string `yaml:"dir"`
	Rsync string `yaml:"rsync"`
}

// GetSites returns all configured sites.
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
	for name, val := range rawMap {
		siteMap, ok := val.(map[string]any)
		if !ok {
			continue
		}
		site := Site{}
		if dir, ok := siteMap["dir"].(string); ok {
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
func GetSite(cfg *Config, name string) (Site, bool) {
	sites := GetSites(cfg)
	site, ok := sites[name]
	return site, ok
}

// AddSite adds or updates a site in the configuration.
func AddSite(cfg *Config, name string, site Site) error {
	if err := Set(cfg, "sites."+name+".dir", site.Dir); err != nil {
		return fmt.Errorf("setting site dir: %w", err)
	}
	if err := Set(cfg, "sites."+name+".rsync", site.Rsync); err != nil {
		return fmt.Errorf("setting site rsync: %w", err)
	}
	return nil
}

// RemoveSite removes a site from the configuration.
func RemoveSite(cfg *Config, name string) error {
	if err := Del(cfg, "sites."+name); err != nil {
		return fmt.Errorf("removing site: %w", err)
	}
	return nil
}
