# Sysmon Host Dashboard Design

## Summary

Add a second provisioned Grafana dashboard focused on `sysmon` host monitoring. This dashboard is separate from the mixed `conmon`/`sysmon` overview page and is optimized for host-specific troubleshooting while still showing a compact fleet-wide summary strip at the top.

## Goals

- Add a dedicated `sysmon` host dashboard
- Keep a small fleet summary visible for context
- Make one selected host the primary focus of the dashboard
- Provide drill-down for host CPU, memory, uptime, boot ID, and per-core CPU
- Provide host-specific service summary and service drill-down panels
- Keep provisioning consistent with the existing Grafana file-based dashboard flow

## Non-Goals

- Replace the existing combined overview dashboard
- Change Grafana provisioning folder structure
- Introduce custom Grafana plugins or frontend code
- Add non-`sysmon` metrics to this dashboard beyond the fleet context panels

## Selected Approach

Add a new dashboard JSON file under the existing provisioned dashboards directory:

- `deploy/grafana/dashboards/sysmon-host.json`

Keep it in the same Grafana folder (`Conmon`) and use tags that make the scope obvious:

- `sysmon`
- `host`
- `operations`

This dashboard should prioritize one selected host, with a small fleet summary row at the top so an operator can keep context while troubleshooting.

## Dashboard Variables

### `host`

- query: `label_values(sysmon_host_info, host)`
- multi-select: disabled
- required host focus
- should default to a concrete host selection rather than `All`

### `service`

- query: `label_values(sysmon_service_active{host=~"$host"}, service)`
- filtered by selected `host`
- multi-select: enabled
- include all: enabled

## Layout

### Row 1: Fleet Summary

This row is intentionally compact. It provides fleet-level context even though the dashboard is host-focused.

Panels:

- stat: host count
  - query: `count(sysmon_host_info)`

- stat: average fleet CPU percent
  - query: `avg(sysmon_host_cpu_usage_ratio) * 100`

- stat: average fleet memory used
  - query: `avg(sysmon_host_memory_resident_bytes)`

- stat: failed service instances across fleet
  - query: `sum(sysmon_service_state{state="failed"})`

- table: host summary
  - fields:
    - `host`
    - `boot_id`
    - uptime
    - CPU percent
    - memory used

These summary panels should ignore the selected `host` variable where appropriate so they remain fleet-wide context.

### Row 2: Host Detail

This row is scoped to the selected host and is the core of the dashboard.

Panels:

- stat: selected host uptime
  - query: `sysmon_host_uptime_seconds{host=~"$host"}`

- stat: selected host boot ID
  - query: `sysmon_host_info{host=~"$host"}`
  - display boot ID from labels or table-style representation

- stat: selected host CPU percent
  - query: `sysmon_host_cpu_usage_ratio{host=~"$host"} * 100`

- stat: selected host memory used
  - query: `sysmon_host_memory_resident_bytes{host=~"$host"}`

- time series: host CPU percent
  - query: `sysmon_host_cpu_usage_ratio{host=~"$host"} * 100`

- time series: host memory used
  - query: `sysmon_host_memory_resident_bytes{host=~"$host"}`

- time series: per-core CPU percent
  - query: `sysmon_host_cpu_core_usage_ratio{host=~"$host"} * 100`

- table: current per-core CPU values
  - query: `sysmon_host_cpu_core_usage_ratio{host=~"$host"} * 100`

## Row 3: Host Services

This row focuses on the currently selected host and its monitored services.

### Summary panels

- stat: monitored services on host
  - query: `count(sysmon_service_active{host=~"$host",service=~"$service"})`

- stat: active services on host
  - query: `sum(sysmon_service_active{host=~"$host",service=~"$service"})`

- stat: enabled services on host
  - query: `sum(sysmon_service_enabled{host=~"$host",service=~"$service"})`

- stat: failed services on host
  - query: `sum(sysmon_service_state{host=~"$host",service=~"$service",state="failed"})`

### Detail panels

- table: service status for selected host
  - fields:
    - `service`
    - current state
    - active
    - enabled
    - uptime
    - CPU usage seconds
    - memory resident bytes

- time series: service CPU by service
  - query: `sysmon_service_cpu_usage_seconds_total{host=~"$host",service=~"$service"}`

- time series: service memory by service
  - query: `sysmon_service_memory_resident_bytes{host=~"$host",service=~"$service"}`

- time series: service uptime by service
  - query: `sysmon_service_uptime_seconds{host=~"$host",service=~"$service"}`

For current state display in the table, the dashboard should use the `sysmon_service_state == 1` series for the state label that is currently active.

## Formatting and Units

- CPU values should be displayed as `0-100` percent
- uptime should use Grafana duration formatting
- memory should use bytes formatting
- boot ID should remain a stat or table field rather than a graph

## Filtering Behavior

- `host` drives all host-detail and host-service panels
- `service` is filtered by the selected `host`
- fleet-summary panels should remain fleet-wide context and not collapse into host-only views

## Implementation Notes

- Add a new dashboard file rather than modifying the existing mixed overview page
- Keep Grafana provisioning unchanged:
  - `deploy/grafana/provisioning/dashboards/dashboard.yml`
- Use the existing Prometheus datasource UID:
  - `prometheus`

## Verification

After implementation:

- validate the dashboard JSON
- confirm that the dashboard provisions cleanly alongside the existing overview dashboard
- ensure the new dashboard references the actual exported metric names:
  - `sysmon_host_cpu_usage_ratio`
  - `sysmon_host_cpu_core_usage_ratio`
  - `sysmon_host_memory_resident_bytes`
  - `sysmon_host_uptime_seconds`
  - `sysmon_host_info`
  - `sysmon_service_uptime_seconds`
  - `sysmon_service_state`
  - `sysmon_service_active`
  - `sysmon_service_enabled`
  - `sysmon_service_cpu_usage_seconds_total`
  - `sysmon_service_memory_resident_bytes`
