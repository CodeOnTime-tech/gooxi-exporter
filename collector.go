package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// --- BMC API Client (per-scrape, no shared state) ---

type bmcClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client

	cookie    string
	csrfToken string
}

func newBMCClient(baseURL, username, password string, insecure bool) *bmcClient {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	return &bmcClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client:   &http.Client{Timeout: 15 * time.Second, Transport: transport},
	}
}

func (c *bmcClient) login(ctx context.Context) error {
	form := url.Values{
		"username": {c.username},
		"password": {c.password},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/session", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("login failed: HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		OK        int    `json:"ok"`
		CSRFToken string `json:"CSRFToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	if result.OK != 0 {
		return fmt.Errorf("login rejected: ok=%d", result.OK)
	}

	for _, ck := range resp.Cookies() {
		if ck.Name == "QSESSIONID" {
			c.cookie = ck.Value
			break
		}
	}
	if c.cookie == "" {
		return fmt.Errorf("QSESSIONID cookie not set")
	}
	c.csrfToken = result.CSRFToken
	return nil
}

func (c *bmcClient) do(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{Name: "QSESSIONID", Value: c.cookie})
	req.Header.Set("X-CSRFTOKEN", c.csrfToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		if err := c.login(ctx); err != nil {
			return nil, fmt.Errorf("re-login failed: %w", err)
		}
		req, _ = http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		req.AddCookie(&http.Cookie{Name: "QSESSIONID", Value: c.cookie})
		req.Header.Set("X-CSRFTOKEN", c.csrfToken)

		resp, err = c.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, body)
	}

	return io.ReadAll(resp.Body)
}

// --- BMC API Response Types ---

type sensor struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Reading     float64 `json:"reading"`
	SensorState int     `json:"sensor_state"`
	Unit        string  `json:"unit"`
}

type chassisStatus struct {
	PowerStatus int `json:"power_status"`
	LEDStatus   int `json:"led_status"`
}

type uptimeResponse struct {
	MinutesPerCount   int `json:"minutes_per_count"`
	POHCounterReading int `json:"poh_counter_reading"`
}

// --- Prometheus Collector ---

type gooxiCollector struct {
	target string
	module string
	config *SafeConfig

	descUp             *prometheus.Desc
	descScrapeDuration *prometheus.Desc
	descSensorValue    *prometheus.Desc
	descSensorState    *prometheus.Desc
	descChassisPower   *prometheus.Desc
	descUptime         *prometheus.Desc
}

func newGooxiCollector(target, module string, config *SafeConfig) *gooxiCollector {
	return &gooxiCollector{
		target: target,
		module: module,
		config: config,
		descUp: prometheus.NewDesc(
			"gooxi_up",
			"Whether the last scrape of the Gooxi BMC was successful.",
			nil, nil,
		),
		descScrapeDuration: prometheus.NewDesc(
			"gooxi_scrape_duration_seconds",
			"Duration of the last Gooxi BMC scrape in seconds.",
			nil, nil,
		),
		descSensorValue: prometheus.NewDesc(
			"gooxi_sensor_value",
			"Sensor reading value from Gooxi BMC.",
			[]string{"name", "type", "unit"}, nil,
		),
		descSensorState: prometheus.NewDesc(
			"gooxi_sensor_state",
			"Sensor state from Gooxi BMC (1=normal, 2=warning, 3=critical).",
			[]string{"name", "type"}, nil,
		),
		descChassisPower: prometheus.NewDesc(
			"gooxi_chassis_power_on",
			"Chassis power state (1=on, 0=off).",
			nil, nil,
		),
		descUptime: prometheus.NewDesc(
			"gooxi_uptime_seconds",
			"System uptime in seconds (from BMC POH counter).",
			nil, nil,
		),
	}
}

func (c *gooxiCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.descUp
	ch <- c.descScrapeDuration
	ch <- c.descSensorValue
	ch <- c.descSensorState
	ch <- c.descChassisPower
	ch <- c.descUptime
}

func (c *gooxiCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	defer func() {
		ch <- prometheus.MustNewConstMetric(
			c.descScrapeDuration, prometheus.GaugeValue,
			time.Since(start).Seconds(),
		)
	}()

	mod := c.config.ModuleFor(c.module)
	baseURL := fmt.Sprintf("https://%s", c.target)
	bmc := newBMCClient(baseURL, mod.Username, mod.Password, mod.Insecure)

	if err := bmc.login(ctx); err != nil {
		logger.Error("login failed", "target", c.target, "module", c.module, "error", err)
		c.emitUp(ch, 0)
		return
	}

	// Fetch everything first, emit metrics only on full success, so a
	// failed scrape never mixes partial data with gooxi_up 0.
	var (
		sensors []sensor
		chassis chassisStatus
		uptime  uptimeResponse
	)

	if sensorData, err := bmc.do(ctx, "/api/sensors"); err != nil {
		logger.Error("fetch sensors failed", "target", c.target, "error", err)
		c.emitUp(ch, 0)
		return
	} else if err := json.Unmarshal(sensorData, &sensors); err != nil {
		logger.Error("parse sensors failed", "target", c.target, "error", err)
		c.emitUp(ch, 0)
		return
	}

	if chassisData, err := bmc.do(ctx, "/api/chassis-status"); err != nil {
		logger.Error("fetch chassis failed", "target", c.target, "error", err)
		c.emitUp(ch, 0)
		return
	} else if err := json.Unmarshal(chassisData, &chassis); err != nil {
		logger.Error("parse chassis failed", "target", c.target, "error", err)
		c.emitUp(ch, 0)
		return
	}

	if uptimeData, err := bmc.do(ctx, "/api/status/uptime"); err != nil {
		logger.Error("fetch uptime failed", "target", c.target, "error", err)
		c.emitUp(ch, 0)
		return
	} else if err := json.Unmarshal(uptimeData, &uptime); err != nil {
		logger.Error("parse uptime failed", "target", c.target, "error", err)
		c.emitUp(ch, 0)
		return
	}

	for _, s := range sensors {
		ch <- prometheus.MustNewConstMetric(
			c.descSensorValue, prometheus.GaugeValue,
			s.Reading, s.Name, s.Type, s.Unit,
		)
		ch <- prometheus.MustNewConstMetric(
			c.descSensorState, prometheus.GaugeValue,
			float64(s.SensorState), s.Name, s.Type,
		)
	}

	ch <- prometheus.MustNewConstMetric(c.descChassisPower, prometheus.GaugeValue, float64(chassis.PowerStatus))

	uptimeSeconds := float64(uptime.POHCounterReading) * float64(uptime.MinutesPerCount) * 60
	ch <- prometheus.MustNewConstMetric(c.descUptime, prometheus.GaugeValue, uptimeSeconds)

	c.emitUp(ch, 1)
}

func (c *gooxiCollector) emitUp(ch chan<- prometheus.Metric, up float64) {
	ch <- prometheus.MustNewConstMetric(c.descUp, prometheus.GaugeValue, up)
}
