package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReloadConfigValid(t *testing.T) {
	path := writeConfig(t, `
modules:
  default:
    username: admin
    password: admin
    insecure: true
  production:
    username: monitor
    password: s3cret
    insecure: false
`)
	sc := &SafeConfig{}
	if err := sc.ReloadConfig(path); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}
	got := sc.ModuleFor("production")
	if got.Username != "monitor" || got.Password != "s3cret" || got.Insecure {
		t.Errorf("production module = %+v", got)
	}
}

func TestReloadConfigAddsMissingDefault(t *testing.T) {
	path := writeConfig(t, `
modules:
  production:
    username: monitor
    password: s3cret
    insecure: true
`)
	sc := &SafeConfig{}
	if err := sc.ReloadConfig(path); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}
	got := sc.ModuleFor("default")
	if got.Username != "admin" || got.Password != "admin" {
		t.Errorf("default module = %+v, want admin/admin fallback", got)
	}
}

func TestModuleForUnknownFallsBackToDefault(t *testing.T) {
	sc := testConfig(map[string]ModuleConfig{
		"default": {Username: "admin", Password: "admin", Insecure: true},
	})
	if got := sc.ModuleFor("nope"); got.Username != "admin" {
		t.Errorf("ModuleFor(nope) = %+v, want default module", got)
	}
}

func TestReloadConfigRejectsUnknownFields(t *testing.T) {
	path := writeConfig(t, `
modules:
  default:
    username: admin
    password: admin
    insecure: true
    bogus: 1
`)
	sc := &SafeConfig{}
	if err := sc.ReloadConfig(path); err == nil {
		t.Fatal("ReloadConfig: expected error for unknown field, got nil")
	}
}

func TestReloadConfigRejectsUnknownTopLevelFields(t *testing.T) {
	path := writeConfig(t, `
bogus: 1
modules:
  default:
    username: admin
    password: admin
    insecure: true
`)
	sc := &SafeConfig{}
	if err := sc.ReloadConfig(path); err == nil {
		t.Fatal("ReloadConfig: expected error for unknown top-level field, got nil")
	}
}

func TestReloadConfigTargetsAndHostModules(t *testing.T) {
	path := writeConfig(t, `
targets:
  - 192.168.0.167
  - 192.168.0.168
host_modules:
  192.168.0.168: production
modules:
  default:
    username: admin
    password: admin
    insecure: true
  production:
    username: monitor
    password: s3cret
    insecure: true
`)
	sc := &SafeConfig{}
	if err := sc.ReloadConfig(path); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}
	if len(sc.C.Targets) != 2 {
		t.Fatalf("targets = %v, want 2 entries", sc.C.Targets)
	}
	if got := sc.C.HostModules["192.168.0.168"]; got != "production" {
		t.Errorf("host_modules[192.168.0.168] = %q, want production", got)
	}
}

func TestModuleForTargetPrecedence(t *testing.T) {
	sc := &SafeConfig{C: &Config{
		Modules: map[string]ModuleConfig{
			"default": {Username: "admin", Password: "admin", Insecure: true},
			"prod":    {Username: "monitor", Password: "s3cret", Insecure: true},
		},
		HostModules: map[string]string{
			"192.168.0.168": "prod",
		},
	}}

	if got := sc.ModuleForTarget("192.168.0.168", "prod"); got.Username != "monitor" {
		t.Errorf("explicit module = %+v, want prod", got)
	}
	if got := sc.ModuleForTarget("192.168.0.168", "typo"); got.Username != "admin" {
		t.Errorf("unknown explicit module = %+v, want default fallback", got)
	}
	if got := sc.ModuleForTarget("192.168.0.168", ""); got.Username != "monitor" {
		t.Errorf("empty module = %+v, want host_modules mapping", got)
	}
	if got := sc.ModuleForTarget("192.168.0.168", "default"); got.Username != "monitor" {
		t.Errorf("default module = %+v, want host_modules mapping", got)
	}
	if got := sc.ModuleForTarget("192.168.0.167", ""); got.Username != "admin" {
		t.Errorf("unmapped target = %+v, want default", got)
	}
}
