# gooxi-exporter

Prometheus exporter for collecting metrics from BCM Gooxi (BMC).

Gooxi BMC does not support Redfish, but exposes its own REST API. The exporter logs in via `POST /api/session`, polls `GET /api/sensors`, and exposes the results as Prometheus metrics.

A single exporter instance collects metrics from **many BMCs** (multi-target pattern): Prometheus passes the BMC address in the `?target=<ip>` query parameter.

## Quick Start (Step by Step)

### Step 1. Get the binary

The binary is a single static file (built with `CGO_ENABLED=0`) and runs on any x86_64 Linux with no dependencies.

Build from source (requires Go 1.23+):

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

## Metrics

| Metric | Description |
|--------|-------------|
| `gooxi_up` | Last scrape success (0/1) |
| `gooxi_scrape_duration_seconds` | Last scrape duration |
| `gooxi_sensor_value{name,type,unit}` | Sensor reading (51 sensors) |
| `gooxi_sensor_state{name,type}` | Sensor state (1=normal, 2=warning, 3=critical) |
| `gooxi_chassis_power_on` | Chassis power state (0/1) |
| `gooxi_uptime_seconds` | System uptime from the BMC POH counter |

## Configuration File

```yaml
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
| `--web.path` | `/metrics` | Metrics endpoint path |

## How It Works

1. Prometheus scrapes `/metrics?target=<bmc-ip>` on the exporter.
2. The exporter looks up credentials in the config module (default: `admin/admin`).
3. Logs in to the BMC: `POST /api/session` (form-encoded) → gets the `QSESSIONID` cookie + `CSRFToken`.
4. Fetches `/api/sensors`, `/api/chassis-status`, `/api/status/uptime` with the cookie and the `X-CSRFTOKEN` header.
5. On `401` — re-logs in and retries.
6. Emits the metrics.

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
