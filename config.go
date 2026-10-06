package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration file structure.
type Config struct {
	Modules map[string]ModuleConfig `yaml:"modules"`

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
