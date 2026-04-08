# conmon

Conmon is a self-hosted connectivity monitoring project intended to run as a small Docker-managed stack behind an internet gateway. The approved deployment model is:

- a `conmon` container built from this repository
- a local Prometheus instance for time-series storage
- a local Grafana instance for dashboards
- a host `systemd` unit, `conmon.service`, that manages the Docker Compose stack

This branch implements the operator layer for that architecture: build targets, install and uninstall flow, Docker Compose assets, Prometheus scrape config, Grafana provisioning, and a `systemd` unit template. The long-running probe engine, metrics exporter, and populated dashboards are still future work. Today the Go entrypoint accepts `-config` and exits after constructing the app shell; it does not yet load the YAML config, run checks, bind a metrics listener, or keep the `conmon` container alive. The documentation below is written to be exact about that current state.

## Overview

The project is organized around one operational entry point: `conmon.service` manages the entire Docker Compose stack rather than a host-run Go daemon. `make install` copies the repository assets into an install root, installs a helper binary, seeds a config file if one does not already exist, renders a systemd unit with the selected paths, and optionally enables and starts the service.

That design keeps the eventual runtime simple to operate:

- operators edit one YAML config file
- `systemctl start|stop|reload conmon` controls the whole stack
- Prometheus stores local metrics data
- Grafana reads from Prometheus and serves the pre-provisioned dashboard

## Current Branch Status

The following pieces are implemented in this worktree right now:

- config schema types and validation rules in `internal/config`
- a buildable Go binary at `cmd/conmon`
- a Dockerfile for packaging the binary
- Make targets for build, install, clean, and uninstall
- Docker Compose, Prometheus, Grafana, and systemd deployment assets

The following pieces are not implemented yet:

- scheduled probe execution
- Prometheus metric export from the `conmon` process
- a long-running `conmon` process that stays up inside the container
- dashboard population with real data

Operational consequence: after install, Prometheus and Grafana can be started as part of the stack layout, but Prometheus will not successfully scrape `conmon` yet and the Grafana panels will stay empty until the runtime work lands.

## Architecture

### Host control plane

The host-level control surface is `conmon.service`. It uses Docker Compose commands to bring the stack up, down, and back into the desired state:

- `ExecStart`: `docker compose -f deploy/docker-compose.yml up -d --build`
- `ExecReload`: `docker compose -f deploy/docker-compose.yml up -d --build`
- `ExecStop`: `docker compose -f deploy/docker-compose.yml down`

This means the service manages the stack as a unit. The helper binary installed to `/usr/local/bin/conmon` is not the primary service entry point; it exists so operators have a locally installed copy of the project binary alongside the containerized stack.

### Containers

`deploy/docker-compose.yml` defines three services:

- `conmon`: built from the source tree copied into the install root, mounts the operator config read-only, and is granted `NET_RAW` so the future ICMP probe implementation can run.
- `prometheus`: scrapes `conmon:9109` on the internal Compose network and stores data in the configured data directory with 90 day retention.
- `grafana`: provisions a Prometheus datasource and a starter dashboard automatically, then serves the UI on `127.0.0.1:3000`.

### Network and data flow

- Prometheus is bound to `127.0.0.1:9091`.
- Grafana is bound to `127.0.0.1:3000`.
- The `conmon` metrics endpoint is published on the host as `0.0.0.0:9109` by default and is also reachable on the internal Compose network at `conmon:9109`.
- Prometheus data persists under `$(DATA_DIR)/prometheus`.
- Grafana data persists under `$(DATA_DIR)/grafana`.

## Prerequisites

You need the following on the target host:

- Linux with `systemd`
- Docker Engine
- Docker Compose v2 plugin, exposed as `docker compose`
- GNU Make
- Go toolchain new enough to build the module declared in `go.mod`

For the default installation flow you also need:

- permission to write `/opt`, `/etc`, `/var/lib`, and `/usr/local/bin`
- permission to install and manage a systemd unit

## Install Layout and Override Variables

The Makefile uses these defaults unless you override them on the command line:

| Variable | Default | Purpose |
| --- | --- | --- |
| `INSTALL_ROOT` | `/opt/conmon` | Installed source tree and Compose working directory |
| `CONFIG_DIR` | `/etc/conmon` | Directory that contains the operator config |
| `CONFIG_FILE` | `/etc/conmon/config.yml` | Config file mounted into the `conmon` container |
| `DATA_DIR` | `/var/lib/conmon` | Persistent Prometheus and Grafana state |
| `BIN_DIR` | `/usr/local/bin` | Install location for the helper binary |
| `SYSTEMD_DIR` | `/etc/systemd/system` | Install location for `conmon.service` |
| `IMAGE_TAG` | `conmon:local` | Local image tag built by `make build` and used by Compose |
| `ENABLE_SYSTEMD` | `1` | When `1`, run `systemctl` actions during install and uninstall |
| `START_SERVICE` | `1` | When `1`, `make install` runs `systemctl enable --now conmon.service` |

Exact paths used by the default install:

- install root: `/opt/conmon`
- config file: `/etc/conmon/config.yml`
- Prometheus data: `/var/lib/conmon/prometheus`
- Grafana data: `/var/lib/conmon/grafana`
- helper binary: `/usr/local/bin/conmon`
- systemd unit: `/etc/systemd/system/conmon.service`

The Compose file itself also honors these environment variables at runtime:

- `CONMON_CONFIG_FILE`
- `CONMON_DATA_DIR`
- `CONMON_IMAGE_TAG`
- `CONMON_METRICS_BIND`

The installed systemd unit sets those environment variables so the Compose stack uses the same paths that were chosen at install time.

`CONMON_METRICS_BIND` controls how the `conmon` metrics port is published on the host. By default it is `0.0.0.0:9109:9109`. You can override it before running `docker compose` if you want to restrict the bind address or move the host port.

## Local Development Build

### `make build`

`make build` performs the two minimum useful build steps for this branch:

1. compile the Go binary to `build/conmon`
2. build a local container image tag, `conmon:local` by default

That gives you:

- a native binary you can inspect or run manually
- a local image tag that the Compose stack can reference immediately

Run it with:

```bash
make build
```

Artifacts:

- `build/conmon`
- Docker image `conmon:local`

### `make clean`

`make clean` removes:

- the local `build/` directory
- the configured local image tag, if it exists

Before removing `$(BUILD_DIR)`, it rejects unsafe values such as empty, `/`, `.`, or `..`.

Run it with:

```bash
make clean
```

## Install

### Default install

```bash
sudo make install
```

### Non-root-safe install for verification

This pattern installs everything into a temporary tree and skips `systemctl` calls:

```bash
make install \
  INSTALL_ROOT="$PWD/.tmp/install-root/opt/conmon" \
  CONFIG_DIR="$PWD/.tmp/install-root/etc/conmon" \
  CONFIG_FILE="$PWD/.tmp/install-root/etc/conmon/config.yml" \
  DATA_DIR="$PWD/.tmp/install-root/var/lib/conmon" \
  BIN_DIR="$PWD/.tmp/install-root/usr/local/bin" \
  SYSTEMD_DIR="$PWD/.tmp/install-root/etc/systemd/system" \
  ENABLE_SYSTEMD=0 \
  START_SERVICE=0
```

### Exactly what `make install` does

`make install` is intentionally explicit. In order, it:

1. Runs preflight checks before any destructive path cleanup:
   - rejects unsafe `INSTALL_ROOT` values such as empty, `/`, `.`, or `..`
   - when `ENABLE_SYSTEMD=1`, verifies that `$(SYSTEMCTL)` is available and tells you to rerun with `ENABLE_SYSTEMD=0` if it is not
2. Runs `make build`, which compiles `build/conmon` and builds the local Docker image tag.
3. Creates the install root, config directory, persistent data directories, helper binary directory, and systemd unit directory.
4. Refreshes the managed application tree in `$(INSTALL_ROOT)` by copying:
   - `Dockerfile`
   - `go.mod`
   - `go.sum`
   - `cmd/`
   - `internal/`
   - `config/`
   - `deploy/`
   - `README.md`
5. Installs the compiled helper binary to `$(BIN_DIR)/conmon`.
6. Installs `config/conmon.example.yml` to `$(CONFIG_FILE)` only if the destination file does not already exist.
7. Preserves an existing `$(CONFIG_FILE)` unchanged if one is already present.
8. Renders `deploy/systemd/conmon.service` into a temporary file in `$(SYSTEMD_DIR)` and atomically renames it into `$(SYSTEMD_DIR)/conmon.service` after the render completes successfully.
9. If `ENABLE_SYSTEMD=1`, runs `systemctl daemon-reload`.
10. If both `ENABLE_SYSTEMD=1` and `START_SERVICE=1`, runs `systemctl enable --now conmon.service`.
11. If `ENABLE_SYSTEMD=0`, skips all `systemctl` actions but still installs the rendered unit file so the layout can be verified in a temporary directory.

Why the source tree is copied into `$(INSTALL_ROOT)` instead of installing only a binary:

- the approved design requires the systemd unit to manage a Docker Compose stack
- the Compose file uses `up -d --build`, so the target host needs the build context, Dockerfile, source tree, and deployment assets available under the install root
- this keeps service control consistent: the host always manages the Compose stack, never a separately launched host binary

### Update flow

To refresh an existing install, run `sudo make install` again. Existing config is preserved, managed files under `$(INSTALL_ROOT)` are replaced, and the systemd unit is re-rendered with the current variable values. Use `START_SERVICE=0` if you want to stage files without enabling or starting the service in the same command.

### What happens today after install

The operator layer is complete, but the application runtime is not. On this branch:

- Prometheus and Grafana assets are installed correctly
- the `conmon` image is built correctly
- the `conmon` container does not stay up because the main program is still only an app shell
- Prometheus therefore cannot scrape `conmon` successfully yet
- the Grafana dashboard is provisioned, but it remains empty

That behavior is expected given the current repository state. It is not a packaging bug in the Makefile or deployment assets.

## Uninstall

### Default uninstall

```bash
sudo make uninstall
```

### What `make uninstall` removes

`make uninstall` removes these managed artifacts:

- `$(SYSTEMD_DIR)/conmon.service`
- `$(INSTALL_ROOT)`
- `$(BIN_DIR)/conmon`

If `ENABLE_SYSTEMD=1`, it also runs:

- `systemctl disable --now conmon.service`
- `systemctl daemon-reload`
- `systemctl reset-failed conmon.service`

Before removing the installed application tree, `make uninstall` performs the same unsafe-path guard on `INSTALL_ROOT` and refuses to continue if the value is empty, `/`, `.`, or `..`. If `ENABLE_SYSTEMD=1`, it also verifies that `$(SYSTEMCTL)` is available instead of silently skipping systemd operations.

### What `make uninstall` preserves

For safety, `make uninstall` preserves by default:

- `$(CONFIG_FILE)`
- `$(DATA_DIR)` and everything under it

That preservation is deliberate so an operator does not accidentally lose configuration or historical Prometheus and Grafana state.

If you truly want a full purge after uninstall, remove those paths manually. With the default layout that would be:

```bash
sudo rm -rf /etc/conmon /var/lib/conmon
```

## Compose and systemd operation

Once installed with the default paths:

- `systemctl start conmon` runs `docker compose -f deploy/docker-compose.yml up -d --build` from `/opt/conmon`
- `systemctl reload conmon` re-runs `docker compose -f deploy/docker-compose.yml up -d --build`, which is how you apply config, provisioning, or image changes without manually stopping the stack first
- `systemctl stop conmon` runs `docker compose -f deploy/docker-compose.yml down`

The unit uses `docker` from systemd's executable search path rather than a hardcoded `/usr/bin/docker`, which makes it more portable across distributions.

If you are not using systemd, you can run the same Compose commands manually from `$(INSTALL_ROOT)` after exporting the same environment variables used by the rendered unit.

## Configuration

### Status of the config layer

The YAML schema described here is implemented in `internal/config`. The main program is not wired to that package yet, so these rules describe the supported configuration format and validation semantics, not a fully wired runtime.

### File format

The operator config is a single YAML document with three top-level sections:

```yaml
defaults:
  interval: 30s
  timeout: 5s
  dns:
    server: 1.1.1.1
  tls:
    min_days_remaining: 21
  labels:
    site: home-lab

groups:
  - name: internet
    checks: []

export:
  listen_address: 0.0.0.0:9109
```

General rules:

- duration values use Go duration strings such as `30s`, `5s`, or `2m`
- the file must be a single YAML document
- unknown YAML fields are rejected
- validation is strict and fails fast

### Top-level sections

#### `defaults`

`defaults` is required and currently supports these keys:

- `interval`
  - type: duration string
  - required: yes
  - validation: must parse and be greater than zero
  - semantics: shared default probe interval
- `timeout`
  - type: duration string
  - required: yes
  - validation: must parse and be greater than zero
  - semantics: shared default probe timeout
- `dns.server`
  - type: string
  - required: no
  - validation: trimmed string, empty allowed
  - semantics: fallback DNS server for DNS checks when the check itself does not set `server`
- `tls.min_days_remaining`
  - type: integer
  - required: no
  - validation: must be non-negative
  - semantics today: validated at the defaults level only; the current loader does not copy it into each `tls` check automatically
- `labels`
  - type: string map
  - required: no
  - validation:
    - keys are trimmed
    - keys must match `^[a-zA-Z_][a-zA-Z0-9_]*$`
    - keys starting with `__` are rejected
    - duplicate keys after trimming are rejected
    - values are trimmed
  - semantics: merged into every check's labels unless overridden by the same key at the check level

#### `groups`

`groups` is required and is a list of check groups.

Each group supports:

- `name`
  - type: string
  - required: yes
  - validation: trimmed, must be non-empty, must be unique across all groups
- `checks`
  - type: list of checks
  - required: yes in practical terms, though an empty list is allowed by the current validator
  - semantics: holds the monitored endpoint definitions

#### `export`

`export` currently supports:

- `listen_address`
  - type: `host:port` string
  - required: yes
  - validation:
    - must parse as `host:port`
    - port must be numeric
    - port must be in the range `1..65535`
  - semantics today: intended future bind address for the Prometheus metrics listener; not yet consumed by `cmd/conmon`

### Shared check fields

Every check supports these base keys:

- `id`
  - required
  - trimmed
  - must be unique across the entire file
- `name`
  - required
  - trimmed
- `kind`
  - required
  - trimmed and normalized to lowercase
  - supported values: `icmp`, `http`, `https`, `tcp`, `dns`, `tls`
- `scope`
  - required
  - trimmed
  - free-form string intended for logical scopes such as `internet`, `external`, or `internal`
- `interval`
  - optional duration string
  - if omitted, null, empty, or `0s`, the loader inherits `defaults.interval`
  - negative values are rejected
- `timeout`
  - optional duration string
  - if omitted, null, empty, or `0s`, the loader inherits `defaults.timeout`
  - negative values are rejected
- `labels`
  - optional string map
  - uses the same key validation rules as `defaults.labels`
  - per-check labels override default labels on matching keys
- `group`
  - not a YAML input field
  - derived internally from the parent `groups[].name`

### Per-kind option reference

#### `icmp`

Required:

- `host`: non-empty string

Optional:

- `count`: integer, must be non-negative

Example:

```yaml
groups:
  - name: internet
    checks:
      - id: gateway-ping
        name: Gateway Reachability
        kind: icmp
        scope: internet
        host: 1.1.1.1
        count: 3
```

#### `http`

Required:

- `url`: absolute URL with scheme `http` and a non-empty host

Optional:

- `method`
  - defaults to `GET`
  - normalized to uppercase
  - must be accepted by Go's HTTP request parser
- `expected_status`
  - exact allow-list of acceptable numeric HTTP status codes
  - defaults to `[200]`
  - every code must be between `100` and `599`
- `headers`
  - string map accepted by the schema
  - runtime semantics are planned but not implemented yet

Example:

```yaml
groups:
  - name: internal-services
    checks:
      - id: internal-http
        name: Internal App
        kind: http
        scope: internal
        url: http://internal-app.local/health
        method: GET
        expected_status: [200]
        headers:
          X-Probe: conmon
```

#### `https`

Required:

- `url`: absolute URL with scheme `https` and a non-empty host

Optional:

- `method`: same rules as `http`
- `expected_status`: same rules as `http`
- `headers`: same rules as `http`
- `tls_server_name`
  - optional string accepted by the schema
  - current state: stored in the config structure, but no runtime behavior is wired yet

Example:

```yaml
groups:
  - name: internet
    checks:
      - id: public-web
        name: Public Web Check
        kind: https
        scope: internet
        url: https://example.com/health
        expected_status: [200, 204]
        tls_server_name: example.com
        headers:
          User-Agent: conmon
```

#### `tcp`

Required:

- `host`: non-empty string
- `port`: integer in the range `1..65535`

Optional:

- only the shared fields described above

Example:

```yaml
groups:
  - name: external-services
    checks:
      - id: api-tcp
        name: Example API
        kind: tcp
        scope: external
        host: api.example.com
        port: 443
```

#### `dns`

Required:

- `query_name`: non-empty string
- `record_type`
  - uppercased by the loader
  - supported values: `A`, `AAAA`, `CNAME`, `MX`, `NS`, `PTR`, `SOA`, `SRV`, `TXT`

Optional:

- `server`
  - if set on the check, it wins
  - otherwise, if `defaults.dns.server` is set, the loader copies that value into the check
  - otherwise, the check is left with an empty server string, which represents system resolver fallback in the future runtime
- `port`
  - defaults to `53` when omitted or set to `0`
  - must be non-negative before defaulting
  - must end up in the range `1..65535`
- `expected_rcode`
  - uppercased by the loader
  - supported values: `NOERROR`, `FORMERR`, `SERVFAIL`, `NXDOMAIN`, `NOTIMP`, `REFUSED`
  - if set, the loader also stores the numeric response-code equivalent internally

Example:

```yaml
groups:
  - name: external-services
    checks:
      - id: public-dns
        name: Public DNS Resolver
        kind: dns
        scope: external
        server: 1.1.1.1
        query_name: example.com
        record_type: A
        expected_rcode: NOERROR
```

#### `tls`

Required:

- `host`: non-empty string
- `port`: integer in the range `1..65535`

Optional:

- `server_name`
  - optional string stored in the config structure
  - runtime behavior is planned but not implemented yet
- `min_days_remaining`
  - optional integer
  - must be non-negative
  - current state: validated on the check when present; the current loader does not default it from `defaults.tls.min_days_remaining`

Example:

```yaml
groups:
  - name: external-services
    checks:
      - id: public-cert
        name: Example Certificate
        kind: tls
        scope: external
        host: example.com
        port: 443
        server_name: example.com
        min_days_remaining: 14
```

### Example full config

`config/conmon.example.yml` contains a complete operator-facing example that exercises every currently supported probe kind and the shared defaults.

## Grafana dashboard and time ranges

The repository provisions a dashboard at `deploy/grafana/dashboards/conmon-overview.json`. It is intentionally simple and references the planned metrics:

- `conmon_probe_success`
- `conmon_probe_duration_seconds`
- `conmon_http_status_code`
- `conmon_dns_rcode`
- `conmon_tls_cert_days_remaining`

The dashboard also provisions template variables for:

- `endpoint_id`
- `group`
- `scope`
- `kind`

Grafana time-range behavior matters because operators will often switch between relative and absolute views:

- relative ranges such as `Last 15 minutes`, `Last 24 hours`, or `now-7d to now` move with the current clock and are useful for live troubleshooting
- absolute ranges pin start and end timestamps and are useful for investigating a known outage window
- the dashboard uses Grafana's built-in time picker, so once the runtime path exports metrics, both range styles will work without any conmon-specific change

On this branch the dashboard is provisioned correctly, but the panels remain empty because the exporter metrics are not emitted yet.

## Repository files added for the operator layer

- `Makefile`: build, install, uninstall, and clean targets
- `deploy/docker-compose.yml`: the Compose stack managed by systemd
- `deploy/prometheus/prometheus.yml`: Prometheus scrape config
- `deploy/grafana/provisioning/datasources/prometheus.yml`: datasource provisioning
- `deploy/grafana/provisioning/dashboards/dashboard.yml`: dashboard provider provisioning
- `deploy/grafana/dashboards/conmon-overview.json`: starter Grafana dashboard
- `deploy/systemd/conmon.service`: systemd unit template rendered by `make install`

## Summary

This branch gives conmon a concrete operator surface:

- you can build the binary and image locally
- you can install the stack into the default system paths or a temporary directory
- you can uninstall the managed artifacts safely without deleting config or data
- you can review the full config schema and the expected deployment layout

What is still missing is the runtime that turns that operator surface into live probe telemetry.
