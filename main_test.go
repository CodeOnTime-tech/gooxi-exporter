package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestValidateWebPath(t *testing.T) {
	valid := []string{"/metrics", "/custom"}
	for _, p := range valid {
		if err := validateWebPath(p); err != nil {
			t.Errorf("validateWebPath(%q) unexpected error: %v", p, err)
		}
	}

	invalid := []string{"", "metrics", "/", "/health", "/discover", "/-/reload", "/health/"}
	for _, p := range invalid {
		if err := validateWebPath(p); err == nil {
			t.Errorf("validateWebPath(%q) expected error, got nil", p)
		}
	}
}

func TestDiscoverHandler(t *testing.T) {
	old := sc
	t.Cleanup(func() { sc = old })
	sc = &SafeConfig{C: &Config{
		Targets: []string{"192.168.0.168", "192.168.0.167", "192.168.0.168"},
		HostModules: map[string]string{
			"192.168.0.168": "production",
			"192.168.0.169": "production",
		},
		Modules: map[string]ModuleConfig{
			"default":    {Username: "admin", Password: "admin", Insecure: true},
			"production": {Username: "monitor", Password: "s3cret", Insecure: true},
		},
	}}

	mux := newMux("/metrics")
	req := httptest.NewRequest(http.MethodGet, "/discover", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var items []discoverItem
	if err := json.NewDecoder(rr.Body).Decode(&items); err != nil {
		t.Fatalf("decode discover response: %v", err)
	}

	want := []struct {
		host   string
		module string
	}{
		{"192.168.0.167", ""},
		{"192.168.0.168", "production"},
		{"192.168.0.169", "production"},
	}
	if len(items) != len(want) {
		t.Fatalf("discover items = %d, want %d", len(items), len(want))
	}
	for i, w := range want {
		if len(items[i].Targets) != 1 || items[i].Targets[0] != w.host {
			t.Errorf("items[%d].targets = %v, want [%s]", i, items[i].Targets, w.host)
		}
		if got := items[i].Labels["module"]; got != w.module {
			t.Errorf("items[%d].labels[module] = %q, want %q", i, got, w.module)
		}
	}
}

func TestDiscoverHandlerEmpty(t *testing.T) {
	old := sc
	t.Cleanup(func() { sc = old })
	sc = &SafeConfig{C: &Config{
		Modules: map[string]ModuleConfig{"default": defaultModule},
	}}

	mux := newMux("/metrics")
	req := httptest.NewRequest(http.MethodGet, "/discover", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rr.Body.String()); got != "[]" {
		t.Errorf("discover response = %q, want \"[]\"", got)
	}
}

func TestHealthHandler(t *testing.T) {
	mux := newMux("/metrics")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != "ok" {
		t.Errorf("health response = %q, want \"ok\"", got)
	}
}

func TestBuildInfoMetric(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(buildInfoCollector{version: "test-version"})

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	mf := family(mfs, "gooxi_exporter_build_info")
	if mf == nil {
		t.Fatal("gooxi_exporter_build_info metric family not found")
	}
	if len(mf.GetMetric()) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(mf.GetMetric()))
	}
	if v := mf.GetMetric()[0].GetGauge().GetValue(); v != 1 {
		t.Errorf("gooxi_exporter_build_info = %v, want 1", v)
	}

	versionLabel := ""
	for _, l := range mf.GetMetric()[0].GetLabel() {
		if l.GetName() == "version" {
			versionLabel = l.GetValue()
		}
	}
	if versionLabel != "test-version" {
		t.Errorf("version label = %q, want test-version", versionLabel)
	}
}
