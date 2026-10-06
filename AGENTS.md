# AGENTS.md

Instructions for AI agents working on this repository.

## Project

Prometheus exporter in Go for collecting metrics from BCM Gooxi (BMC). Gooxi BMC does not have a Redfish API but provides its own REST API. Uses the **multi-target exporter pattern**: one exporter instance scrapes many BMCs via `?target=<host>`.

## Architecture

- **Language**: Go 1.23+
- **Paradigm**: single binary, HTTP server, multi-target via query param
- **Key dependencies**: `github.com/prometheus/client_golang`, `gopkg.in/yaml.v3`
- **BMC API**: HTTPS, cookie + CSRF header auth
- **Config**: YAML file with "modules" (credential groups), SIGHUP reload

### File layout

```
main.go              — entry point, flags, HTTP server, multi-target routing, SIGHUP
collector.go         — prometheus.Collector, BMC HTTP client, sensor parsing
config.go            — YAML config loading, SafeConfig (RWMutex), module lookup
config.example.yml   — example configuration file
```

## BMC API (Gooxi)

### Authentication Flow

1. `POST /api/session` with **form-encoded** body (`username=...&password=...`)
   - Response: JSON `{"ok":0, "CSRFToken":"...", "privilege":4, ...}`
   - Sets cookie: `QSESSIONID=<value>; path=/; secure`
2. All subsequent requests require **both**:
   - Cookie: `QSESSIONID=<value>`
   - Header: `X-CSRFTOKEN: <CSRFToken from login response>`
3. On `401` — re-login (CSRF token changes each session)

**Important:** The login endpoint accepts `application/x-www-form-urlencoded`, NOT JSON. Sending JSON returns 500.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/session` | Login (form-encoded). Returns `CSRFToken` in JSON body |
| GET | `/api/sensors` | All sensors (JSON array, 51 items) |
| GET | `/api/detail_sensors_readings` | Detailed sensor readings |
| GET | `/api/chassis-status` | Chassis state: `{"power_status":1,"led_status":0}` |
| GET | `/api/status/uptime` | Uptime: `{"minutes_per_count":60,"poh_counter_reading":N}` |
| DELETE | `/api/session` | Logout |

### Sensor JSON Structure

```json
{
  "id": 1,
  "sensor_number": 1,
  "name": "CPU0_Temp",
  "type": "temperature",
  "reading": 42.0,
  "sensor_state": 1,
  "unit": "deg_c"
}
```

Note: threshold fields (`lower_critical_threshold`, etc.) can be numeric OR the string `"NA"`. Do not parse them as float64.

Sensor types: `temperature`, `voltage`, `fan`, `power_supply`, `power_unit`, `current`, `processor`, `cooling_device`, `physical_security`

## Multi-Target Pattern

- Prometheus scrapes `/metrics?target=<bmc-host>&module=<module-name>`
- The `target` param is the BMC address (IP or hostname)
- The `module` param selects a credential group from the config (default: `"default"`)
- Each scrape creates a fresh BMC client (no shared session state between targets)
- Prometheus relabels `__address__` → `__param_target` (see README for examples)

## Conventions

- **Metrics**: prefix `gooxi_`, units implied by context
- **Errors**: if BMC is unreachable — `gooxi_up 0`, do not crash
- **Logging**: `log/slog` (structured), no external deps
- **Flags**: standard `flag` package
- **TLS**: `insecure: true` in config (BMCs use self-signed certs)
- **Config**: YAML, validated on load, hot-reloadable via SIGHUP or `POST /-/reload`

## Commands

```bash
go build -o gooxi-exporter .    # build
go vet ./...                     # static analysis
go test ./...                    # tests
gofmt -w .                       # formatting
```

## Rules

1. Do not add heavy dependencies without need. `client_golang` + `yaml.v3` + stdlib is sufficient.
2. The exporter must be resilient: HTTP timeouts, retry on 401, graceful degradation per-target.
3. Never hardcode credentials — config file only (with safe defaults of admin/admin).
4. Before committing: `gofmt`, `go vet`, `go build` must pass.
5. Metrics use `prometheus.Desc` / `prometheus.MustNewConstMetric` (const metrics pattern).
6. Always send both `QSESSIONID` cookie AND `X-CSRFTOKEN` header on API requests.
7. Each scrape is independent: create a new BMC client per scrape, do not pool sessions.

## Do NOT

- Do not use Redfish libraries — Gooxi has no Redfish.
- Do not do long-polling or WebSocket — pull on scrape only.
- Do not cache metrics between scrapes (each scrape = fresh request to BMC).
- Do not send JSON to `/api/session` — it must be form-encoded.
- Do not share BMC client state between concurrent scrapes of different targets.
