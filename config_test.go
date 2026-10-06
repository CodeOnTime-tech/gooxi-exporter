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
