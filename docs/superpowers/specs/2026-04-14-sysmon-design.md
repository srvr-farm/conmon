# Sysmon Host Monitoring Design

## Summary

Add a second service named `sysmon` to this repository. `sysmon` is a Linux-only host daemon that collects local system resource metrics and selected `systemd` unit metrics, then pushes them into the same monitoring stack used by `conmon`.

The central `conmon` deployment remains the operator-managed stack and is extended with a Pushgateway instance. Each host that runs `sysmon` pushes metrics to that shared Pushgateway over a trusted local network without authentication.

This design keeps `conmon` focused on connectivity checks while adding a separate, independently installable daemon for host and service telemetry.

## Goals

- Add a new service named `sysmon`
- Keep `sysmon` operationally separate from `conmon`
- Push `sysmon` metrics into the same central monitoring stack already used by `conmon`
- Support running `sysmon` on multiple Linux hosts in the local network
- Include a `host` dimension on all `sysmon` metrics
- Default the `host` dimension to the local hostname with an optional per-host override
- Monitor host resident memory usage, total CPU usage, per-core CPU usage, uptime, and boot ID
- Monitor a configured allowlist of `systemd` services
- For each configured `systemd` unit, report uptime, state, active status, enabled status, CPU usage, and resident memory usage for the whole unit including child processes
- Provide a separate `make install-sysmon` flow so `sysmon` can be installed independently on remote hosts

## Non-Goals for V1

- Windows, macOS, or non-Linux support
- Support for service managers other than `systemd`
- TLS or authentication for the `sysmon` push endpoint
- Automatic expiry or garbage collection of stale host metrics in Pushgateway
- Alerting rules or dashboard redesign beyond the minimum deployment changes needed for ingestion
- General process monitoring outside configured `systemd` units

## Selected Architecture

The repository will contain two monitoring services with different roles:

1. `conmon` remains the central connectivity-monitoring service
2. `sysmon` becomes a host-local resource-monitoring daemon

The central Docker Compose stack managed by `conmon.service` will be extended to include:

1. `conmon`
2. `prometheus`
3. `grafana`
4. `pushgateway`

`conmon` continues to expose scrape-based metrics directly to Prometheus. `sysmon` does not get scraped directly by Prometheus. Instead, each `sysmon` instance pushes metrics to Pushgateway, and Prometheus scrapes Pushgateway.

This architecture is not the preferred Prometheus model for continuously changing host metrics, but it is the selected design because the required operating model is active push from multiple remote hosts.

## Deployment Model

### Central Stack

The existing central stack remains installed through `make install` and managed by `conmon.service`. That install path gains Pushgateway and the Prometheus scrape configuration needed to ingest pushed `sysmon` metrics.

Central stack responsibilities:

- run `conmon`
- run Prometheus
- run Grafana
- run Pushgateway
- expose Pushgateway on the LAN so remote `sysmon` daemons can push to it

### Remote Hosts

Each monitored Linux host installs `sysmon` as a normal host daemon through a separate `make install-sysmon` target. `sysmon` is not managed by the central Docker Compose stack.

Remote host responsibilities:

- install the `sysmon` binary
- install a local `sysmon` config file
- install and manage `sysmon.service`
- read local `/proc`, `systemd`, and cgroup state
- push metrics to the central Pushgateway on a fixed interval

## Configuration Model

`sysmon` reads its own YAML configuration file. The format is intentionally separate from `conmon` configuration because the runtime concerns are different.

### Top-Level Sections

The v1 config contains these concern areas:

1. Push settings
2. Host identity settings
3. Host collection settings
4. `systemd` service allowlist

### Proposed Shape

```yaml
push:
  url: http://monitoring-gateway.lan:9092
  job: sysmon
  interval: 30s
  timeout: 5s

identity:
  host: ""

system:
  collect_per_core_cpu: true

services:
  - name: sshd.service
  - name: docker.service
  - name: nginx.service
```

### Field Semantics

#### `push`

- `url`: required Pushgateway base URL
- `job`: optional Pushgateway job name, default `sysmon`
- `interval`: required collection and push interval
- `timeout`: required timeout applied to collection and push operations

#### `identity`

- `host`: optional override for the emitted `host` label
- if empty, `sysmon` resolves the local hostname from the operating system and uses that value

#### `system`

- `collect_per_core_cpu`: optional boolean, default `true`

#### `services`

- explicit allowlist of `systemd` unit names to monitor
- each entry requires `name`
- v1 does not support wildcards, globbing, or automatic discovery

### Validation Rules

`sysmon` config should fail fast on startup with actionable errors when:

- `push.url` is empty or invalid
- `push.interval` is not greater than zero
- `push.timeout` is not greater than zero
- `push.job` is empty after trimming
- `identity.host` is present but empty after trimming
- a service name is empty
- a service name is duplicated

## Metric Model

All `sysmon` metrics must include a `host` label. The `host` value comes from:

1. `identity.host`, if configured
2. otherwise the local hostname

Service-specific metrics also include a `service` label naming the `systemd` unit.

### Host Metrics

- `sysmon_host_cpu_usage_ratio`
  - labels: `host`
  - gauge
  - latest whole-host CPU usage as a ratio from `0` to `1`

- `sysmon_host_cpu_core_usage_ratio`
  - labels: `host`, `core`
  - gauge
  - latest per-core CPU usage ratio from `0` to `1`

- `sysmon_host_memory_resident_bytes`
  - labels: `host`
  - gauge
  - host memory usage approximated as `MemTotal - MemAvailable`

- `sysmon_host_uptime_seconds`
  - labels: `host`
  - gauge
  - current host uptime in seconds

- `sysmon_host_info`
  - labels: `host`, `boot_id`
  - gauge
  - constant value `1`
  - carries boot identity as metadata

### Service Metrics

- `sysmon_service_uptime_seconds`
  - labels: `host`, `service`
  - gauge
  - seconds since the unit entered active state
  - `0` when the unit is not active

- `sysmon_service_state`
  - labels: `host`, `service`, `state`
  - gauge
  - state-set metric for the current unit state

- `sysmon_service_active`
  - labels: `host`, `service`
  - gauge with `1` for active, `0` for not active

- `sysmon_service_enabled`
  - labels: `host`, `service`
  - gauge with `1` for enabled, `0` for not enabled

- `sysmon_service_cpu_usage_seconds_total`
  - labels: `host`, `service`
  - counter-like gauge sourced from cgroup CPU accounting for the entire unit

- `sysmon_service_memory_resident_bytes`
  - labels: `host`, `service`
  - gauge
  - resident memory usage for the full unit cgroup

### State Encoding

For `sysmon_service_state`, the implementation should emit a small stable set of known state label values such as:

- `active`
- `inactive`
- `failed`
- `activating`
- `deactivating`

Only the current state should carry value `1`. Emitted non-current known states should carry `0` so dashboards and alerts can query state transitions cleanly.

## Collection Design

### Host Data Sources

Use Linux kernel interfaces directly where practical:

- `/proc/stat` for total and per-core CPU counters
- `/proc/meminfo` for host memory usage
- `/proc/uptime` for uptime
- `/proc/sys/kernel/random/boot_id` for boot ID

CPU usage requires interval-to-interval delta calculation. `sysmon` should keep the previous `/proc/stat` sample in memory and compute ratios on each cycle.

### Service Data Sources

Use `systemd` and cgroup data together:

- `systemctl show <unit>` for unit state, active status, enabled status, timestamps, and cgroup path
- cgroup files for CPU and memory accounting for the full service unit including child processes

The implementation should avoid reporting only the main PID. The service CPU and memory metrics must reflect the unit cgroup as a whole.

### Cgroup Behavior

The collector should support common Linux cgroup layouts used on current distributions. The design target is:

- resolve the unit cgroup path from `systemctl show`
- read CPU usage from the appropriate cgroup accounting file
- read memory usage from the appropriate cgroup accounting file

The implementation plan should isolate cgroup file resolution behind a narrow interface so tests can cover both cgroup v1-style and v2-style file layouts where needed.

### Uptime Semantics for Services

When a service is active, `sysmon_service_uptime_seconds` is computed from the unit active-enter timestamp reported by `systemd`.

When a service is inactive or failed:

- `sysmon_service_active` becomes `0`
- `sysmon_service_uptime_seconds` becomes `0`
- state and enabled metrics continue to report current values

## Push Model

`sysmon` owns an in-process Prometheus registry and updates that registry on each collection cycle. After metrics are refreshed, `sysmon` pushes the registry contents to Pushgateway using:

- job name from `push.job`
- grouping key `host=<resolved-or-overridden-host>`

The implementation should push full current-state metrics on every cycle rather than only changed values.

### Stale Metric Handling

Stale metrics are intentionally not cleaned up automatically in v1.

If a host disappears unexpectedly, its last pushed metrics remain in Pushgateway until an operator removes them manually. This is an accepted trade-off for the selected push-based operating model.

## Build, Install, and Service Management

### Central Install

`make install` remains responsible for the central monitoring stack. It should be extended to:

- build any central artifacts needed for the updated stack
- install updated Compose assets including Pushgateway
- install updated Prometheus configuration that scrapes both `conmon` and Pushgateway
- preserve existing `conmon` config behavior

### Independent Sysmon Install

Add new Make targets:

- `make install-sysmon`
- `make uninstall-sysmon`

`make install-sysmon` should:

- build `cmd/sysmon`
- install the binary to a default host path such as `/usr/local/bin/sysmon`
- create the `sysmon` config directory if needed
- install `config/sysmon.example.yml` to the destination config path only when it does not already exist
- render and install `deploy/systemd/sysmon.service`
- optionally run `systemctl daemon-reload`
- optionally enable and start `sysmon.service`

`make uninstall-sysmon` should:

- stop and disable `sysmon.service` when systemd management is enabled
- remove the installed `sysmon` unit
- remove the installed `sysmon` binary
- preserve existing config unless explicitly designed otherwise

### Default Sysmon Paths

Proposed defaults:

- config directory: `/etc/sysmon`
- config file: `/etc/sysmon/config.yml`
- binary: `/usr/local/bin/sysmon`
- unit file: `/etc/systemd/system/sysmon.service`

These paths should be Makefile-overridable in the same style used by the existing `conmon` install flow.

## Repository Layout

The implementation should keep `sysmon` logic separate from existing `conmon` probe logic.

Expected additions:

- `cmd/sysmon/main.go`
- `internal/sysmon/config/...`
- `internal/sysmon/collector/...`
- `internal/sysmon/push/...`
- `internal/sysmon/metrics/...`
- `deploy/systemd/sysmon.service`
- `config/sysmon.example.yml`

Expected modifications:

- `Makefile`
- `deploy/docker-compose.yml`
- `deploy/prometheus/prometheus.yml`
- `README.md`

## Testing Strategy

V1 requires automated tests for:

- `sysmon` config parsing and validation
- hostname resolution and override behavior
- CPU usage delta calculations from `/proc/stat` samples
- parsing `systemctl show` output into normalized service status
- cgroup CPU and memory reading logic using fixture files
- metric registration and emitted labels
- Pushgateway request construction
- Makefile install rendering behavior where existing tests or lightweight command verification can cover it

Tests should avoid depending on a live `systemd` instance by abstracting command execution and filesystem reads behind small interfaces.

## Operational Notes

- The monitored hosts must be able to reach the central Pushgateway over the LAN
- The central Pushgateway bind address should be configurable
- Because the transport is unauthenticated in v1, this deployment is appropriate only for a trusted local network
- Dashboards and alert rules should prefer the `host` label rather than inferring origin from job names or scrape targets

## Open Questions Deferred from V1

- Whether `sysmon` should delete its Pushgateway group on clean shutdown
- Whether to add Pushgateway stale-series cleanup tooling
- Whether to add HTTPS and basic auth support for the push endpoint
- Whether to surface additional host metadata labels such as kernel version or OS release
- Whether to support automatic service discovery or wildcard unit selection
