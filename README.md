# gooxi-exporter

Prometheus exporter for collecting metrics from BCM Gooxi (BMC).

Gooxi BMC does not support Redfish, but exposes its own REST API. The exporter logs in via `POST /api/session`, polls `GET /api/sensors`, and exposes the results as Prometheus metrics.

A single exporter instance collects metrics from **many BMCs** (multi-target pattern): Prometheus passes the BMC address in the `?target=<ip>` query parameter.

## Quick Start (Step by Step)

### Step 1. Get the binary

The binary is a single static file (built with `CGO_ENABLED=0`) and runs on any x86_64 Linux with no dependencies.

Prebuilt binaries are available in the [GitHub Releases](https://github.com/CodeOnTime-tech/gooxi-exporter/releases).

Or build from source (requires Go 1.23+):

```bash
make build
```

This produces the `gooxi-exporter` file in the project root.

### Step 2. Install on the server

On the server that will perform the scraping (usually the Prometheus host or a dedicated monitoring host):

```bash
sudo useradd --system --user-group --home-dir /opt/gooxi --shell /usr/sbin/nologin gooxi
sudo cp gooxi-exporter /opt/gooxi/
sudo chown -R gooxi:gooxi /opt/gooxi
```

### Step 3. Configuration

If all BMCs use `admin` / `admin`, a minimal config is enough:

```bash
sudo mkdir -p /etc/gooxi
sudo tee /etc/gooxi/config.yml > /dev/null <<'EOF'
modules:
  default:
    username: admin
    password: admin
    insecure: true
EOF
sudo chown root:gooxi /etc/gooxi/config.yml
sudo chmod 640 /etc/gooxi/config.yml
```

You can skip the file entirely — the exporter starts with the built-in `admin/admin` defaults. But an explicit file is better: it is easy to edit and won't be lost.

If different BMC groups use different credentials, add modules (see [Configuration File](#configuration-file)).

### Step 4. Run as a systemd service

```bash
sudo cp systemd/gooxi-exporter.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now gooxi-exporter
systemctl status gooxi-exporter
```

The exporter now:

- starts on server boot;
- restarts on crash;
- runs in the background — **you can log off the server**.

### Step 5. Verify

```bash
curl -s "http://localhost:9108/metrics?target=192.168.0.167" | head
```

Expected: `gooxi_up 1` and `gooxi_sensor_value{...}` metrics.

If you see `gooxi_up 0`, check the logs:

```bash
journalctl -u gooxi-exporter -n 50 --no-pager
```

Common causes: BMC not reachable over the network, port 443 blocked by a firewall, wrong credentials.

### Step 6. Configure Prometheus

Add to `/etc/prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: gooxi
    metrics_path: /metrics
    params:
      target: ['__address__']        # BMC address goes into ?target=
    static_configs:
      - targets:
          - 192.168.0.167
          - 192.168.0.168
          - 192.168.0.169
    relabel_configs:
      # 1) remember the BMC address as the instance label
      - source_labels: [__address__]
        target_label: instance
      # 2) point the scrape at the machine running the exporter
      - target_label: __address__
        replacement: 10.0.0.10:9108
```

`10.0.0.10:9108` — the address and port of the machine running the exporter.

Reload Prometheus: `sudo systemctl reload prometheus`. In the web UI, the **Targets** page should show all BMCs as `UP`.

#### Many BMCs — file_sd

Keep the BMC list in a separate file so you don't touch the Prometheus config when adding servers.

`/etc/prometheus/gooxi-targets.yml`:

```yaml
- targets:
    - 192.168.0.167
    - 192.168.0.168
  labels:
    job: gooxi
```

`prometheus.yml`:

```yaml
scrape_configs:
  - job_name: gooxi
    metrics_path: /metrics
    file_sd_configs:
      - files: ['/etc/prometheus/gooxi-targets.yml']
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 10.0.0.10:9108
```

#### Dynamic target list — http_sd

The exporter can serve the BMC list directly from `/discover`:

`/etc/gooxi/config.yml`:

```yaml
targets:
  - 192.168.0.167
  - 192.168.0.168

host_modules:
  192.168.0.168: production
```

`prometheus.yml`:

```yaml
scrape_configs:
  - job_name: gooxi
    metrics_path: /metrics
    http_sd_configs:
      - url: http://10.0.0.10:9108/discover
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__address__]
        target_label: instance
      - source_labels: [__meta_http_sd_module]
        target_label: __param_module
      - target_label: __address__
        replacement: 10.0.0.10:9108
```

`10.0.0.10:9108` is the address of the machine running the exporter. The optional `module` label from `/discover` is passed to the exporter as `?module=`.

#### Different credentials per group — modules

Declare modules in the exporter config (see below) and pass them via `params.module` in Prometheus:

```yaml
scrape_configs:
  - job_name: gooxi-default
    metrics_path: /metrics
    params:
      module: ['default']
    static_configs:
      - targets: [192.168.0.167, 192.168.0.168]
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 10.0.0.10:9108

  - job_name: gooxi-production
    metrics_path: /metrics
    params:
      module: ['production']
    static_configs:
      - targets: [10.0.1.50, 10.0.1.51]
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 10.0.0.10:9108
```

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/metrics?target=<host>` | Everything (backward compatible) |
| `/metrics/sensors?target=<host>` | Sensor readings and states only |
| `/metrics/health?target=<host>` | BMC chassis power and uptime only |
| `/discover` | JSON target list for Prometheus `http_sd_configs` |
| `/health` | Exporter liveness probe |

All scrape endpoints also accept `&module=<name>` to select a credential group.

Note: `/health` reports the exporter process, while `/metrics/health` reports BMC chassis health.

Scrape the fast-changing sensor data often and the slow-changing health data less often:

```yaml
scrape_configs:
  - job_name: gooxi-sensors
    metrics_path: /metrics/sensors
    scrape_interval: 15s
    # ...targets and relabeling as below

  - job_name: gooxi-health
    metrics_path: /metrics/health
    scrape_interval: 5m
    # ...same targets
```

## Metrics

All metrics carry a `host` label with the target address.

| Metric | Description |
|--------|-------------|
| `gooxi_up{host}` | Last scrape success (0/1) |
| `gooxi_scrape_duration_seconds{host}` | Last scrape duration |
| `gooxi_sensor_value{host,name,type,unit}` | Sensor reading (51 sensors) |
| `gooxi_sensor_state{host,name,type}` | Sensor state (1=normal, 2=warning, 3=critical) |
| `gooxi_chassis_power_on{host}` | Chassis power state (0/1) |
| `gooxi_uptime_seconds{host}` | System uptime from the BMC POH counter |
| `gooxi_exporter_build_info{version}` | Exporter build information |

## Configuration File

```yaml
# Optional: expose these hosts on /discover for Prometheus http_sd.
targets:
  - 192.168.0.167
  - 192.168.0.168

# Optional: select a credential module per target.
# Used when ?module= is not provided, and exposed as a label in /discover.
host_modules:
  192.168.0.168: production

modules:
  default:            # used when ?module= is not passed
    username: admin
    password: admin
    insecure: true    # BMCs use self-signed certificates

  production:         # named group for BMCs with different credentials
    username: monitor
    password: s3cret
    insecure: true
```

- The module is selected by the `?module=<name>` parameter; the default is `default`.
- If `?module=` is absent or `default`, a `host_modules` mapping for the target is used when present.
- `/discover` returns the sorted unique union of `targets` and `host_modules` keys.
- If `default` is not declared, `admin/admin` is substituted automatically.
- The config is picked up on the fly, without a restart:

```bash
sudo systemctl kill -s HUP gooxi-exporter
# or
curl -X POST http://localhost:9108/-/reload
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config.file` | — | Path to the YAML config file |
| `--listen` | `:9108` | Listen address |
| `--web.path` | `/metrics` | Metrics endpoint path (category endpoints are `<path>/sensors` and `<path>/health`; must not collide with `/health`, `/discover`, or `/-/reload`) |
| `--timeout` | `20s` | Timeout for a single BMC scrape |
| `--log.level` | `info` | Log level: debug, info, warn, error |
| `--version` | — | Print version and exit |

## How It Works

1. Prometheus scrapes `/metrics?target=<bmc-ip>` on the exporter.
2. The exporter looks up credentials in the config module (default: `admin/admin`).
3. Logs in to the BMC: `POST /api/session` (form-encoded) → gets the `QSESSIONID` cookie + `CSRFToken`.
4. Fetches `/api/sensors`, `/api/chassis-status`, `/api/status/uptime` with the cookie and the `X-CSRFTOKEN` header.
5. On `401` — re-logs in and retries.
6. Emits the metrics (only if every request succeeded — a failed scrape returns `gooxi_up 0` and no partial data).
7. Closes the BMC session: `DELETE /api/session`.

Each scrape is independent: a fresh BMC client is created per scrape, and sessions are never shared between targets.

## Discovering BMCs on the Network

```bash
./scripts/discover.sh 192.168.0.0/24
```

The script finds Gooxi BMCs in a subnet (open HTTPS port + JSON response from `/api/session`) and prints the list of IPs — copy it into the Prometheus `targets`.

## Docker

```dockerfile
FROM golang:1.23 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /gooxi-exporter .

FROM alpine:3
COPY --from=builder /gooxi-exporter /gooxi-exporter
EXPOSE 9108
USER nobody
ENTRYPOINT ["/gooxi-exporter"]
```

```bash
docker build -t gooxi-exporter .
docker run -d --restart=unless-stopped --name gooxi-exporter \
  -p 9108:9108 \
  -v /etc/gooxi/config.yml:/config.yml:ro \
  gooxi-exporter --config.file=/config.yml
```

## Building & Development

```bash
make        # gofmt + go vet + go build
make test   # tests
make vet    # static analysis
```

## License

MIT
