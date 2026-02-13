# Tempest Exporter

A Prometheus exporter for [WeatherFlow Tempest](https://weatherflow.com/tempest-weather-system/) weather stations. Connects via WebSocket for real-time observations and exposes them as Prometheus metrics.

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [API Usage and Rate Limits](#api-usage-and-rate-limits)
- [Quick Start](#quick-start)
  - [Finding Your Device ID and Station ID](#finding-your-device-id-and-station-id)
  - [Configuration](#configuration)
  - [Run Locally](#run-locally)
  - [Run with Docker](#run-with-docker)
  - [Deploy to Kubernetes](#deploy-to-kubernetes)
- [Metrics](#metrics)
  - [Observation Metrics](#observation-metrics)
  - [Exporter Health](#exporter-health)
- [HTTP Endpoints](#http-endpoints)
- [Example PromQL Queries](#example-promql-queries)
- [Derived Metric Formulas](#derived-metric-formulas)
- [Grafana Dashboard](#grafana-dashboard)
- [Building](#building)
- [License](#license)

## Features

- Real-time observations via WebSocket (~60s intervals)
- REST API fallback when WebSocket is disconnected for >5 minutes
- 17 observation metrics + 2 derived metrics (dew point, feels like) + event tracking
- Derived metrics computed locally (Magnus formula for dew point, wind chill/heat index for feels like)
- Lightning strike and rain start event tracking
- Health endpoints for Kubernetes liveness and readiness probes
- Multi-arch container images (linux/amd64, linux/arm64) via ko

## Architecture

```
                    ┌────────────────────────────┐
                    │   Tempest WebSocket API    │
                    │  wss://ws.weatherflow.com  │
                    └───────────┬────────────────┘
                                │ obs_st every ~60s
                                │ evt_strike, evt_precip
                                ▼
                    ┌───────────────────────────┐
                    │   tempest-exporter        │
                    │                           │
                    │  WebSocket client ────────│──► internal gauges/counters
                    │  (reconnect on failure)   │
                    │                           │
                    │  GET /metrics ◄───────────│─── prometheus.Handler()
                    │  GET /healthz             │
                    │  GET /readyz              │
                    └───────────┬───────────────┘
                                │ :8080/metrics
                                ▼
                    ┌─────────────────────────┐
                    │   VictoriaMetrics       │
                    │   (scrapes every 60s)   │
                    └─────────────────────────┘
```

The exporter maintains a persistent WebSocket connection to `wss://ws.weatherflow.com/swd/data`. When the connection drops, it reconnects with exponential backoff (1s to 60s). If disconnected for more than 5 minutes, it falls back to polling the REST API every 60 seconds.

A custom Prometheus collector computes derived metrics (dew point, feels like) at scrape time from the latest observation snapshot. Concurrency is handled with a `sync.RWMutex` — the WebSocket goroutine writes, and Prometheus scrape reads.

## Prerequisites

- [Go](https://go.dev/) 1.24+
- [ko](https://ko.build/) (for building container images)
- [kubectl](https://kubernetes.io/docs/tasks/tools/) (for Kubernetes deployment)
- A WeatherFlow Tempest weather station
- A WeatherFlow API token ([get one here](https://tempestwx.com/settings/tokens))

## API Usage and Rate Limits

### How This Exporter Uses the API

This exporter is **read-only** and makes minimal use of the WeatherFlow API:

- **1 persistent WebSocket connection** to `wss://ws.weatherflow.com/swd/data` — receives observations pushed by the server every ~60 seconds
- **REST fallback only** — if the WebSocket is disconnected for >5 minutes, polls `swd.weatherflow.com` at most once per minute until the WebSocket reconnects

### Rate Limits

WeatherFlow enforces the following rate limits **per user** (shared across all tokens belonging to your account):

| Resource | Limit |
|----------|-------|
| WebSocket connections | 10 concurrent |
| REST API requests | 100 per minute |

### Important Considerations

- **Run only 1 replica per token.** Each instance opens its own WebSocket connection, counting against the 10-connection limit. If you run other integrations on the same account (Home Assistant, Tempest app, etc.), they share the same limits.
- **The API token is free** for personal use with your own station, but there is no SLA on the WebSocket API. Expect occasional disconnections.
- The most common cause of rate limit errors is failing to close connections before reconnecting. This exporter handles reconnection automatically with exponential backoff.

### Network Requirements

The exporter requires outbound access on port 443 to:

| Host | Protocol | Purpose |
|------|----------|---------|
| `ws.weatherflow.com` | WSS | Real-time observations |
| `swd.weatherflow.com` | HTTPS | REST API fallback |

## Quick Start

### Finding Your Device ID and Station ID

You can find your device and station IDs using the Tempest REST API:

```bash
# List your stations (replace YOUR_TOKEN)
curl -s "https://swd.weatherflow.com/swd/rest/stations?token=YOUR_TOKEN" | jq '.stations[] | {station_id: .station_id, name: .name, devices: [.devices[] | {device_id: .device_id, serial_number: .serial_number}]}'
```

### Configuration

Via environment variables (set in K8s deployment):

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TEMPEST_TOKEN` | Yes | | WeatherFlow API token |
| `TEMPEST_DEVICE_ID` | Yes | | Device ID for WebSocket subscription |
| `TEMPEST_STATION_ID` | Yes | | Station ID for REST fallback |
| `TEMPEST_STATION_NAME` | No | `tempest` | Human-readable name, used as `station_name` metric label |
| `LISTEN_ADDR` | No | `:8080` | HTTP listen address |

### Run Locally

```bash
export TEMPEST_TOKEN="your-token"
export TEMPEST_DEVICE_ID="your-device-id"
export TEMPEST_STATION_ID="your-station-id"
go run .
```

Then visit `http://localhost:8080/metrics`.

### Run with Docker

```bash
docker run -e TEMPEST_TOKEN=xxx -e TEMPEST_DEVICE_ID=xxx -e TEMPEST_STATION_ID=xxx \
  -p 8080:8080 ghcr.io/chadmayfield/tempest-exporter:latest
```

### Deploy to Kubernetes

1. Create the namespace:

```bash
kubectl apply -f deploy/namespace.yaml
```

2. Create the secret (token only — never stored in a file):

```bash
kubectl create secret generic tempest-exporter-token \
  --namespace monitoring \
  --from-literal=TEMPEST_TOKEN=your-actual-token
```

3. Update `deploy/configmap.yaml` with your device and station IDs:

```bash
kubectl create configmap tempest-exporter-config \
  --namespace monitoring \
  --from-literal=TEMPEST_DEVICE_ID=your-device-id \
  --from-literal=TEMPEST_STATION_ID=your-station-id \
  --from-literal=TEMPEST_STATION_NAME=your-station-name \
  --from-literal=LISTEN_ADDR=:8080
```

Or edit `deploy/configmap.yaml` directly and apply it — it contains no secrets.

4. Apply manifests:

```bash
kubectl apply -f deploy/
```

Or build and deploy with ko:

```bash
export KO_DOCKER_REPO=ghcr.io/youruser  # or your registry
ko apply -f deploy/
```

## Metrics

### Observation Metrics

All metrics carry labels: `station_id`, `station_name` (values from env vars).

| Metric | Type | Description |
|--------|------|-------------|
| `tempest_air_temperature_celsius` | gauge | Air temperature (Celsius) |
| `tempest_feels_like_temperature_celsius` | gauge | Feels-like temperature (derived) |
| `tempest_dew_point_celsius` | gauge | Dew point via Magnus formula |
| `tempest_relative_humidity_percent` | gauge | Relative humidity (%) |
| `tempest_station_pressure_millibars` | gauge | Station pressure (millibars) |
| `tempest_wind_speed_meters_per_second` | gauge | Average wind speed (m/s) |
| `tempest_wind_gust_meters_per_second` | gauge | Wind gust speed (m/s) |
| `tempest_wind_lull_meters_per_second` | gauge | Wind lull speed (m/s) |
| `tempest_wind_direction_degrees` | gauge | Wind direction (degrees) |
| `tempest_solar_radiation_watts` | gauge | Solar radiation (W/m2) |
| `tempest_uv_index` | gauge | UV index |
| `tempest_illuminance_lux` | gauge | Illuminance (lux) |
| `tempest_precipitation_millimeters` | gauge | Precipitation accumulation (mm) |
| `tempest_precipitation_type` | gauge | Precipitation type (0=none, 1=rain, 2=hail, 3=rain+hail) |
| `tempest_lightning_strike_distance_kilometers` | gauge | Average lightning strike distance (km) |
| `tempest_lightning_strike_count` | gauge | Lightning strike count |
| `tempest_battery_volts` | gauge | Battery voltage |
| `tempest_rain_start_epoch_seconds` | gauge | Unix timestamp of last rain start event |

### Exporter Health

| Metric | Type | Description |
|--------|------|-------------|
| `tempest_up` | gauge | 1=WebSocket connected, 0=disconnected |
| `tempest_last_observation_timestamp_seconds` | gauge | Epoch of last obs_st received |
| `tempest_websocket_reconnects_total` | counter | Total reconnection attempts |
| `tempest_scrape_errors_total` | counter | Errors serving /metrics |

## HTTP Endpoints

| Endpoint | Description |
|----------|-------------|
| `/metrics` | Prometheus metrics |
| `/healthz` | Liveness probe (always 200 if process alive) |
| `/readyz` | Readiness probe (200 after first observation, 503 before) |

## Example PromQL Queries

Do **not** export daily high/low/avg from the stats endpoint. Prometheus and Grafana compute these natively:

```promql
# Daily high temperature
max_over_time(tempest_air_temperature_celsius[24h])

# Daily low temperature
min_over_time(tempest_air_temperature_celsius[24h])

# Daily average temperature
avg_over_time(tempest_air_temperature_celsius[24h])
```

## Derived Metric Formulas

**Dew Point** (Magnus formula):
```
gamma = (17.27 * T) / (237.7 + T) + ln(RH / 100)
dew_point = (237.7 * gamma) / (17.27 - gamma)
```

**Feels Like**:
- Wind chill (T < 10C, wind > 4.8 km/h): Environment Canada formula
- Heat index (T >= 27C, RH >= 40%): Rothfusz/NOAA regression
- Otherwise: air temperature

## Grafana Dashboard

A pre-built Grafana dashboard is included at `grafana/dashboard.json`. Import it into your Grafana instance for panels covering temperature, humidity, wind, pressure, solar, precipitation, lightning, battery, and exporter health.

<!-- TODO: Add dashboard screenshot -->

## Building

```bash
# Binary
go build -o tempest-exporter .

# Container image
ko build --bare .

# Build and deploy to K8s
ko apply -f deploy/

# Run tests
go test ./... -race -count=1
```

## License

MIT
