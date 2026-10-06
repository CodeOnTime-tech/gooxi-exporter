package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestMain(m *testing.M) {
	logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	os.Exit(m.Run())
}

const (
	testSessionID = "sess-test-123"
	testCSRFToken = "csrf-test-456"
)

// mockBMC simulates the Gooxi BMC REST API over TLS.
type mockBMC struct {
	t        *testing.T
	server   *httptest.Server
	username string
	password string

	sensors []sensor
	chassis chassisStatus
	uptime  uptimeResponse

	loginCalls   int
	logoutCalls  int
	unauthorized bool // next data request returns 401 once
	failSensors  bool // /api/sensors returns 500
}

func newMockBMC(t *testing.T) *mockBMC {
	t.Helper()
	m := &mockBMC{
		t:        t,
		username: "admin",
		password: "admin",
		sensors: []sensor{
			{Name: "CPU0_Temp", Type: "temperature", Reading: 42.5, SensorState: 1, Unit: "deg_c"},
			{Name: "SYS_Fan1", Type: "fan", Reading: 3200, SensorState: 1, Unit: "rpm"},
		},
		chassis: chassisStatus{PowerStatus: 1, LEDStatus: 0},
		uptime:  uptimeResponse{MinutesPerCount: 60, POHCounterReading: 123},
	}
	m.server = httptest.NewTLSServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

// target returns the host:port part of the mock server URL.
func (m *mockBMC) target() string {
	return strings.TrimPrefix(m.server.URL, "https://")
}

func (m *mockBMC) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/session":
		m.loginCalls++
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("username") != m.username || form.Get("password") != m.password {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "QSESSIONID", Value: testSessionID, Path: "/"})
		json.NewEncoder(w).Encode(map[string]any{"ok": 0, "CSRFToken": testCSRFToken})

	case r.Method == http.MethodDelete && r.URL.Path == "/api/session":
		m.logoutCalls++
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodGet:
		cookie, err := r.Cookie("QSESSIONID")
		if err != nil || cookie.Value != testSessionID || r.Header.Get("X-CSRFTOKEN") != testCSRFToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if m.unauthorized {
			m.unauthorized = false
			http.Error(w, "session expired", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/sensors":
			if m.failSensors {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(m.sensors)
		case "/api/chassis-status":
			json.NewEncoder(w).Encode(m.chassis)
		case "/api/status/uptime":
			json.NewEncoder(w).Encode(m.uptime)
		default:
			http.NotFound(w, r)
		}

	default:
		http.NotFound(w, r)
	}
}

func testConfig(modules map[string]ModuleConfig) *SafeConfig {
	return &SafeConfig{C: &Config{Modules: modules}}
}

func defaultTestConfig() *SafeConfig {
	return testConfig(map[string]ModuleConfig{
		"default": {Username: "admin", Password: "admin", Insecure: true},
	})
}

// gather runs the collector and returns the gathered metric families.
func gather(t *testing.T, c *gooxiCollector) []*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	return mfs
}

func family(mfs []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

func gaugeValue(t *testing.T, mf *dto.MetricFamily) float64 {
	t.Helper()
	if mf == nil {
		t.Fatal("metric family not found")
	}
	if len(mf.GetMetric()) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(mf.GetMetric()))
	}
	return mf.GetMetric()[0].GetGauge().GetValue()
}

func labeledGauge(t *testing.T, mf *dto.MetricFamily, label, value string) float64 {
	t.Helper()
	if mf == nil {
		t.Fatal("metric family not found")
	}
	for _, m := range mf.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == label && l.GetValue() == value {
				return m.GetGauge().GetValue()
			}
		}
	}
	t.Fatalf("sample with %s=%q not found", label, value)
	return 0
}

func sampleCount(mfs []*dto.MetricFamily, name string) int {
	mf := family(mfs, name)
	if mf == nil {
		return 0
	}
	return len(mf.GetMetric())
}

func TestCollectAll(t *testing.T) {
	mock := newMockBMC(t)
	c := newGooxiCollector(mock.target(), "default", categoryAll, defaultTestConfig(), 10*time.Second)

	mfs := gather(t, c)

	if v := gaugeValue(t, family(mfs, "gooxi_up")); v != 1 {
		t.Errorf("gooxi_up = %v, want 1", v)
	}
	if v := labeledGauge(t, family(mfs, "gooxi_sensor_value"), "name", "CPU0_Temp"); v != 42.5 {
		t.Errorf("CPU0_Temp = %v, want 42.5", v)
	}
	if v := labeledGauge(t, family(mfs, "gooxi_sensor_state"), "name", "SYS_Fan1"); v != 1 {
		t.Errorf("SYS_Fan1 state = %v, want 1", v)
	}
	if v := gaugeValue(t, family(mfs, "gooxi_chassis_power_on")); v != 1 {
		t.Errorf("chassis power = %v, want 1", v)
	}
	if v := gaugeValue(t, family(mfs, "gooxi_uptime_seconds")); v != 123*60*60 {
		t.Errorf("uptime = %v, want %v", v, 123*60*60)
	}
	if mock.logoutCalls != 1 {
		t.Errorf("logoutCalls = %d, want 1", mock.logoutCalls)
	}
}

func TestCollectSensorsOnly(t *testing.T) {
	mock := newMockBMC(t)
	c := newGooxiCollector(mock.target(), "default", categorySensors, defaultTestConfig(), 10*time.Second)

	mfs := gather(t, c)

	if v := gaugeValue(t, family(mfs, "gooxi_up")); v != 1 {
		t.Errorf("gooxi_up = %v, want 1", v)
	}
	if n := sampleCount(mfs, "gooxi_sensor_value"); n != 2 {
		t.Errorf("sensor samples = %d, want 2", n)
	}
	if family(mfs, "gooxi_chassis_power_on") != nil {
		t.Error("chassis metric present in sensors-only scrape")
	}
	if family(mfs, "gooxi_uptime_seconds") != nil {
		t.Error("uptime metric present in sensors-only scrape")
	}
}

func TestCollectHealthOnly(t *testing.T) {
	mock := newMockBMC(t)
	c := newGooxiCollector(mock.target(), "default", categoryHealth, defaultTestConfig(), 10*time.Second)

	mfs := gather(t, c)

	if v := gaugeValue(t, family(mfs, "gooxi_up")); v != 1 {
		t.Errorf("gooxi_up = %v, want 1", v)
	}
	if n := sampleCount(mfs, "gooxi_sensor_value"); n != 0 {
		t.Errorf("sensor samples = %d, want 0 in health-only scrape", n)
	}
	if v := gaugeValue(t, family(mfs, "gooxi_chassis_power_on")); v != 1 {
		t.Errorf("chassis power = %v, want 1", v)
	}
	if v := gaugeValue(t, family(mfs, "gooxi_uptime_seconds")); v != 123*60*60 {
		t.Errorf("uptime = %v, want %v", v, 123*60*60)
	}
}

func TestCollectLoginFailure(t *testing.T) {
	mock := newMockBMC(t)
	cfg := testConfig(map[string]ModuleConfig{
		"default": {Username: "wrong", Password: "wrong", Insecure: true},
	})
	c := newGooxiCollector(mock.target(), "default", categoryAll, cfg, 10*time.Second)

	mfs := gather(t, c)

	if v := gaugeValue(t, family(mfs, "gooxi_up")); v != 0 {
		t.Errorf("gooxi_up = %v, want 0", v)
	}
	if n := sampleCount(mfs, "gooxi_sensor_value"); n != 0 {
		t.Errorf("sensor samples = %d, want 0", n)
	}
	if mock.logoutCalls != 0 {
		t.Errorf("logoutCalls = %d, want 0 (no session was opened)", mock.logoutCalls)
	}
}

func TestCollectModuleSelection(t *testing.T) {
	mock := newMockBMC(t)
	mock.username = "monitor"
	mock.password = "s3cret"
	cfg := testConfig(map[string]ModuleConfig{
		"default": {Username: "admin", Password: "admin", Insecure: true},
		"prod":    {Username: "monitor", Password: "s3cret", Insecure: true},
	})
	c := newGooxiCollector(mock.target(), "prod", categoryAll, cfg, 10*time.Second)

	mfs := gather(t, c)

	if v := gaugeValue(t, family(mfs, "gooxi_up")); v != 1 {
		t.Errorf("gooxi_up = %v, want 1 (module 'prod' credentials should be used)", v)
	}
}

func TestCollectReLoginOn401(t *testing.T) {
	mock := newMockBMC(t)
	mock.unauthorized = true
	c := newGooxiCollector(mock.target(), "default", categoryAll, defaultTestConfig(), 10*time.Second)

	mfs := gather(t, c)

	if v := gaugeValue(t, family(mfs, "gooxi_up")); v != 1 {
		t.Errorf("gooxi_up = %v, want 1 after re-login", v)
	}
	if mock.loginCalls != 2 {
		t.Errorf("loginCalls = %d, want 2 (initial + re-login)", mock.loginCalls)
	}
}

func TestCollectSensorsFailureNoPartialMetrics(t *testing.T) {
	mock := newMockBMC(t)
	mock.failSensors = true
	c := newGooxiCollector(mock.target(), "default", categoryAll, defaultTestConfig(), 10*time.Second)

	mfs := gather(t, c)

	if v := gaugeValue(t, family(mfs, "gooxi_up")); v != 0 {
		t.Errorf("gooxi_up = %v, want 0", v)
	}
	if n := sampleCount(mfs, "gooxi_sensor_value"); n != 0 {
		t.Errorf("sensor samples = %d, want 0 on failed scrape", n)
	}
	if n := sampleCount(mfs, "gooxi_chassis_power_on"); n != 0 {
		t.Errorf("chassis samples = %d, want 0 on failed scrape", n)
	}
}
