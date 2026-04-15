I'm using the writing-plans skill to create the implementation plan.

# Sysmon Deployment Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update the README with the new sysmon deployment model, capture the verification steps, and keep the central stack description current.

**Architecture:** Document how the central Compose stack now pushes host metrics through Pushgateway while sysmon runs as a remote host daemon with its own install flow, and detail how Prometheus, Make targets, and host labels coalesce in that flow.

**Tech Stack:** Go/Make, Prometheus, README documentation, systemd/Compose deployment scripts.

---

### Task 1: README coverage

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Capture the missing guidance**
  Run `rg -n "Pushgateway|install-sysmon|sysmon.service|host override" README.md` to demonstrate the file currently lacks the new sysmon deployment guidance referenced in Task 7.
  Expected: the command exits non-zero and prints no matches, creating a clear failing checklist item.

- [ ] **Step 2: Add the required documentation sections**
  Extend README with concise subsections covering:
  1. The central stack architecture that includes Pushgateway and how Prometheus scrapes sysmon metrics pushed through it.
  2. The remote `sysmon` host daemon model, including how it differs from the Compose stack.
  3. The difference between `make install` and `make install-sysmon`, emphasizing separate install targets.
  4. How `host` labels are resolved and how remote hosts can override defaults.
  5. A sample remote-host `sysmon` configuration and example install commands demonstrating overrides.
  Keep the writing operational, matching the repo's documentation tone.

- [ ] **Step 3: Verify the guidance now exists**
  Re-run `rg -n "Pushgateway|install-sysmon|sysmon.service|host override" README.md`.
  Expected: the command succeeds with each key term appearing in the new sections.

- [ ] **Step 4: Review the README diff**
  Run `git diff README.md` to ensure only the new sections were added and there are no unrelated changes or formatting regressions.

- [ ] **Step 5: Stage and capture the doc change**
  Run `git add README.md` and `git commit -m "docs: document sysmon deployment"`.

### Task 2: Verification and coupling check

**Files:**
- Not modifying files, but verifying generated artifacts and diffs.

**Test:** `go test ./...`, `make build`, `make build-sysmon`, central install command from Task 7 (conditionally), and the prescribed diff checks.

- [ ] **Step 1: Run Go unit tests**
  `go test ./...`
  Expected: passes without failures.

- [ ] **Step 2: Build `conmon`**
  `make build`
  Expected: `build/conmon` exists after the command.

- [ ] **Step 3: Build `sysmon`**
  `make build-sysmon`
  Expected: `build/sysmon` exists.

- [ ] **Step 4: Optional central install (if Docker is available)**
  Run the provided `make install` command with the `.tmp/central` directories and flags from Task 7.
  Expected: install tree under `.tmp/central` contains the updated Compose and Prometheus assets, and systemd units are staged as configured.

- [ ] **Step 5: Review diffs for accidental coupling**
  Run:
  ```
  git diff --stat
  git diff -- README.md Makefile deploy/docker-compose.yml deploy/prometheus/prometheus.yml deploy/systemd/sysmon.service cmd/sysmon internal/sysmon
  ```
  Expected: only README plus the intentional central stack/sysmon files appear (no unrelated files changed).
