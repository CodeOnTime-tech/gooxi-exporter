package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration file structure.
type Config struct {
	Targets     []string                `yaml:"targets"`
	HostModules map[string]string       `yaml:"host_modules"`
	Modules     map[string]ModuleConfig `yaml:"modules"`

	XXX map[string]any `yaml:",inline"`
}

// ModuleConfig holds credentials and settings for a group of BMCs.
type ModuleConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Insecure bool   `yaml:"insecure"`

	XXX map[string]any `yaml:",inline"`
}

// SafeConfig provides concurrency-safe access to the configuration.
type SafeConfig struct {
	sync.RWMutex
	C *Config
}

var defaultModule = ModuleConfig{
	Username: "admin",
	Password: "admin",
	Insecure: true,
}

func checkOverflow(m map[string]any, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return checkOverflow(c.XXX, "config")
}

func (m *ModuleConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*m = defaultModule
	type plain ModuleConfig
	if err := unmarshal((*plain)(m)); err != nil {
		return err
	}
	return checkOverflow(m.XXX, "module")
}

// ReloadConfig reads and parses the config file in a concurrency-safe way.
func (sc *SafeConfig) ReloadConfig(path string) error {
	var c Config

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
		if err := yaml.Unmarshal(data, &c); err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	} else {
		c.Modules = map[string]ModuleConfig{"default": defaultModule}
	}

	if _, ok := c.Modules["default"]; !ok {
		c.Modules["default"] = defaultModule
	}

	sc.Lock()
	sc.C = &c
	sc.Unlock()
	return nil
}

// ModuleFor returns the module config for a given module name, falling back to "default".
func (sc *SafeConfig) ModuleFor(module string) ModuleConfig {
	sc.RLock()
	defer sc.RUnlock()

	if module != "" && module != "default" {
		if m, ok := sc.C.Modules[module]; ok {
			return m
		}
	}
	return sc.C.Modules["default"]
}

// ModuleForTarget resolves the credential module for a scrape target.
// An explicit non-default module wins; otherwise a host_modules mapping is
// used; otherwise the default module is returned.
func (sc *SafeConfig) ModuleForTarget(target, requested string) ModuleConfig {
	sc.RLock()
	defer sc.RUnlock()

	if requested != "" && requested != "default" {
		if m, ok := sc.C.Modules[requested]; ok {
			return m
		}
		return sc.C.Modules["default"]
	}

	if m, ok := sc.C.HostModules[target]; ok && m != "" {
		if mod, ok := sc.C.Modules[m]; ok {
			return mod
		}
	}

	return sc.C.Modules["default"]
}

// discoverItem is one entry of the /discover JSON response.
type discoverItem struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// DiscoverItems returns the sorted unique list of targets advertised on
// /discover. The list is the union of the top-level targets and the
// host_modules keys.
func (sc *SafeConfig) DiscoverItems() []discoverItem {
	sc.RLock()
	defer sc.RUnlock()

	hostModule := make(map[string]string)
	for _, host := range sc.C.Targets {
		if host == "" {
			continue
		}
		if _, exists := hostModule[host]; !exists {
			hostModule[host] = ""
		}
	}
	for host, module := range sc.C.HostModules {
		if host == "" {
			continue
		}
		if _, exists := hostModule[host]; !exists {
			hostModule[host] = ""
		}
		if module != "" && module != "default" {
			hostModule[host] = module
		}
	}

	hosts := make([]string, 0, len(hostModule))
	for host := range hostModule {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)

	items := make([]discoverItem, 0, len(hosts))
	for _, host := range hosts {
		item := discoverItem{Targets: []string{host}}
		if module := hostModule[host]; module != "" {
			item.Labels = map[string]string{"module": module}
		}
		items = append(items, item)
	}
	return items
}
