# Systemd Status and Cgroup Usage Design

## Summary

Add a dedicated `internal/sysmon/systemd` package that parses `systemctl show`
output into a normalized service status and reads cgroup CPU + memory usage for a
unit. The package feeds sysmon’s existing metrics pipeline with deterministic
inputs so the upper layers stay testable and easy to reason about.

## Context

This work continues the host monitoring collector from the April 14 design by
handling the `systemd` data sources that `sysmon` needs for service-specific
metrics. The collection conductor already knows how to iterate tracked unit names
and push metrics; this package simply supplies the data it needs:

- A `UnitStatus` representing the latest `systemctl show` fields that matter
  (name, state, enabled flag, control group, monotonic enter timestamp).
- A `Usage` snapshot of the unit cgroup’s CPU and memory residency.

All data must be gatherable from in-memory inputs so the unit tests can use
`strings.Join`, `fstest.MapFS`, or synthetic values rather than a real `systemd`
instance or live `/sys/fs/cgroup` tree.

## Requirements

1. Parse `systemctl show <unit>` output into a `UnitStatus` with:
   - `Name`, `State`, `Active`, `Enabled`, `ControlGroup`,
     `ActiveEnterMonotonicUS`.
   - `Active` derived from known state values (`active`, `activating`, etc.).
   - `Enabled` derived from `UnitFileState` (enabled, linked => true, others
     false).
   - Normalized parsing helpers for `Id`, `ActiveState`, `UnitFileState`,
     `ActiveEnterTimestampMonotonic`, and `ControlGroup`.
2. Surface helpers:
   - `KnownStateValues() []string` (used by callers to know allowed state labels).
   - `EnabledFromUnitFileState(state string) bool`.
   - A helper that computes service uptime (seconds) when provided the current
     monotonic timestamp.
3. Read cgroup usage via `ReadUsage(fs fs.FS, controlGroup string) (Usage, error)`:
   - Prefer cgroup v2 layout under `/sys/fs/cgroup/<controlGroup>` reading
     `cpu.stat` (`usage_usec`) and `memory.current`.
   - Fall back to cgroup v1 controllers, i.e. `/sys/fs/cgroup/cpuacct/<controlGroup>`
     for `cpuacct.usage` (nanoseconds) and
     `/sys/fs/cgroup/memory/<controlGroup>` for `memory.usage_in_bytes`.
   - Normalize to `Usage{CPUUsageSecondsTotal float64, MemoryResidentBytes uint64}`
     regardless of which controller succeeded first.

## Approach Options

1. **Manual switch parser (preferred).** Read `systemctl` output line by line, split
   on `=` and assign by literal keys. This keeps the package small, testable, and
   easy to extend when new fields are needed. Helpers live alongside the parser.
2. **Reflection/tagged struct parser.** Use a struct with `systemd` tags and iterate
   through fields to reduce boilerplate. It still needs explicit conversions for
   booleans and timestamps, so the added abstraction outweighs the benefit for
   this handful of fields.
3. **Map-backed parser.** Build a `map[string]string` first, then validate keys.
   This gives more room for future consumers but adds a second pass and increases
   the parsing surface area. It also complicates testing of the normalized helpers.

Recommendation: go with the manual switch parser (option 1). It keeps the parsing
in one place, leaves the normalized helpers explicit, and minimizes the surface
area that needs testing.

## Chosen Design

### UnitStatus parser

- Implement `ParseStatus(data []byte) (UnitStatus, error)` that trims each line,
  splits at the first `=` and assigns values inside a `switch`.
- Track `ActiveState`, `UnitFileState`, and `ControlGroup` verbatim, use
  `ActiveState` to set `State` and `Active` (`Active` is `true` when the state
  equals `active`).
- Parse `ActiveEnterTimestampMonotonic` into `ActiveEnterMonotonicUS` (default `0`
  when missing). Treat unparsable numbers as errors so upstream consumers know
  when the timestamp cannot be trusted.
- Export `ActiveEnterMonotonicUS` as a `uint64` and build `ComputeUptimeSeconds`
  that takes the current monotonic microseconds and returns `float64` seconds.
- `KnownStateValues()` returns `[]string{"active","inactive","failed","activating","deactivating"}`
  so callers can populate state label sets.

### Cgroup usage reader

- The caller passes the control group path (starting with `/`). `ReadUsage` joins
  `sys/fs/cgroup` with the trimmed path to locate cgroup v2 files.
- For CPU, it first tries `cpu.stat`, parses lines until it finds `usage_usec` and
  converts to seconds. For memory it reads `memory.current` as `uint64`.
- If either v2 file is missing or invalid, attempt v1: read
  `/sys/fs/cgroup/cpuacct/<controlGroup>/cpuacct.usage` and convert nanoseconds
  to seconds, and `/sys/fs/cgroup/memory/<controlGroup>/memory.usage_in_bytes`.
- Return `Usage` once both CPU and memory are resolved; if a file is missing from
  both layouts, return a wrapped error to pin down which controller failed.
- Keep arithmetic deterministic so tests can evaluate floating-point conversions
  exactly (`4_200_000 µs => 4.2s`, `42_000_000_000 ns => 42s`).

## Testing Strategy

- `ParseStatus` tests cover:
  - simple active unit (provided by the failing test in the task). Validate `Active``true`, `Enabled` from `UnitFileState`, and `ActiveEnterMonotonicUS` parsing.
  - unknown state (should still populate `State`).
  - `KnownStateValues` contains the expected entries.
  - `EnabledFromUnitFileState` recognizes `enabled` and `linked` as truthy.
  - `ComputeUptimeSeconds` handles `now < enter` gracefully.
- `ReadUsage` tests use `fstest.MapFS` fixtures for both cgroup v2 and v1:
  - v2 fixture with `cpu.stat` (usage_usec) and `memory.current` verifying 4.2s CPU.
  - v1 fixture with `cpuacct.usage` and `memory.usage_in_bytes` verifying nanosecond conversion and fallback logic.
  - missing files produce clear errors.

## Next Steps

1. Push this spec to `docs/superpowers/specs/2026-04-15-systemd-status-design.md` (done).
2. Dispatch the spec document reviewer subagent using the provided template.
3. After reviewer approval, ask the user to confirm the spec before moving to
   implementation and invoke the `writing-plans` skill.

Once the spec is approved, TDD flow: add the failing tests, implement parser +
cgroup reader, run `go test ./internal/sysmon/systemd -v`, and keep the helper
functions deterministic so callers may provide the current monotonic timestamp.
