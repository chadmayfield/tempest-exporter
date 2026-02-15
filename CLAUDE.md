# tempest-exporter

Go Prometheus exporter for WeatherFlow Tempest weather stations.

## Build & Run

- Build tool: `ko` (https://ko.build) — no Dockerfile
- Build image: `ko build --bare .`
- Deploy to K8s: `ko apply -f deploy/`
- Run locally: `go run .` (requires TEMPEST_TOKEN, TEMPEST_DEVICE_ID, TEMPEST_STATION_ID env vars)
- Run tests: `go test ./...`
- Run tests with race detector: `go test -race ./...`
- Lint: `golangci-lint run`

## Conventions

- All station-specific config comes from environment variables — never hardcode station IDs, tokens, or names
- Metric names follow Prometheus naming: `tempest_` prefix, snake_case, base unit suffix (_celsius, _meters_per_second, _volts, etc.)
- All metrics carry `station_id` and `station_name` labels
- WebSocket is the primary data source; REST API is fallback/supplement only
- K8s manifests live in `deploy/` — secret.yaml is a template, never commit real tokens
- Container base image: `cgr.dev/chainguard/static` (distroless)
- Target platforms: linux/amd64, linux/arm64

## Working Style

- Ask for clarification instead of guessing — especially for registry, K8s context, module path, and design choices
- Verify assumptions: check that tools (ko, kubectl, golangci-lint) are available before using them
- Run tests after writing or modifying code
- Never commit secrets, tokens, or real credentials
- Explain tradeoffs briefly when making design decisions so the user can course-correct
- NEVER tag/release without end-to-end testing first — build, run against live service, verify output
- When told to verify: STOP changing code. Read source, trace paths, confirm with evidence. Only then propose a fix
- Trace code paths step by step through actual source — don't guess what functions do
- Verify claims through multiple independent means before presenting them as fact
- Separate investigation from implementation — understand the full problem before proposing a fix
