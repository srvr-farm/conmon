SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

GO ?= go
DOCKER ?= docker
INSTALL ?= install
SYSTEMCTL ?= systemctl

BUILD_DIR ?= build
BINARY := $(BUILD_DIR)/conmon
SYSMON_BINARY := $(BUILD_DIR)/sysmon
IMAGE_TAG ?= conmon:local

INSTALL_ROOT ?= /opt/conmon
CONFIG_DIR ?= /etc/conmon
CONFIG_FILE ?= $(CONFIG_DIR)/config.yml
DATA_DIR ?= /var/lib/conmon
BIN_DIR ?= /usr/local/bin
HELPER_BIN ?= $(BIN_DIR)/conmon
GRAFANA_ADMIN_HELPER ?= $(BIN_DIR)/conmon-grafana-admin
SYSTEMD_DIR ?= /etc/systemd/system
UNIT_FILE ?= $(SYSTEMD_DIR)/conmon.service

SYSMON_CONFIG_DIR ?= /etc/sysmon
SYSMON_CONFIG_FILE ?= $(SYSMON_CONFIG_DIR)/config.yml
SYSMON_BIN_DIR ?= /usr/local/bin
SYSMON_HELPER_BIN ?= $(SYSMON_BIN_DIR)/sysmon
SYSMON_SYSTEMD_DIR ?= /etc/systemd/system
SYSMON_UNIT_FILE ?= $(SYSMON_SYSTEMD_DIR)/sysmon.service

ENABLE_SYSTEMD ?= 1
START_SERVICE ?= 1
PROMETHEUS_UID ?= 65534
PROMETHEUS_GID ?= 65534
GRAFANA_UID ?= 472
GRAFANA_GID ?= 472

APP_TREE_ITEMS := Dockerfile go.mod go.sum cmd internal config deploy scripts README.md

define require_safe_rm_path
case "$(strip $(1))" in ""|"/"|"."|"..") printf '%s\n' "Refusing destructive operation with $(2)=$(strip $(1)). Choose a non-empty path that is not /, . or .." >&2; exit 1;; esac
endef

.PHONY: build build-sysmon install install-sysmon uninstall uninstall-sysmon clean preflight-install preflight-install-sysmon preflight-uninstall preflight-uninstall-sysmon preflight-clean verify-task6-install

verify-task6-install:
	$(call require_safe_rm_path,$(CURDIR)/.tmp,VERIFY_TMP_ROOT)
	rm -rf "$(CURDIR)/.tmp"
	$(MAKE) install \
	  INSTALL_ROOT="$(CURDIR)/.tmp/central/opt/conmon" \
	  CONFIG_DIR="$(CURDIR)/.tmp/central/etc/conmon" \
	  CONFIG_FILE="$(CURDIR)/.tmp/central/etc/conmon/config.yml" \
	  DATA_DIR="$(CURDIR)/.tmp/central/var/lib/conmon" \
	  BIN_DIR="$(CURDIR)/.tmp/central/usr/local/bin" \
	  SYSTEMD_DIR="$(CURDIR)/.tmp/central/etc/systemd/system" \
	  ENABLE_SYSTEMD=0 \
	  START_SERVICE=0
	test -f "$(CURDIR)/.tmp/central/opt/conmon/deploy/docker-compose.yml"
	test -f "$(CURDIR)/.tmp/central/etc/systemd/system/conmon.service"
	$(MAKE) install-sysmon \
	  ENABLE_SYSTEMD=0 \
	  START_SERVICE=0 \
	  SYSMON_CONFIG_DIR="$(CURDIR)/.tmp/sysmon/etc/sysmon" \
	  SYSMON_CONFIG_FILE="$(CURDIR)/.tmp/sysmon/etc/sysmon/config.yml" \
	  SYSMON_BIN_DIR="$(CURDIR)/.tmp/sysmon/usr/local/bin" \
	  SYSMON_SYSTEMD_DIR="$(CURDIR)/.tmp/sysmon/etc/systemd/system"
	test -f "$(CURDIR)/.tmp/sysmon/usr/local/bin/sysmon"
	test -f "$(CURDIR)/.tmp/sysmon/etc/sysmon/config.yml"
	test -f "$(CURDIR)/.tmp/sysmon/etc/systemd/system/sysmon.service"
	if grep -q '@SYSMON_HELPER_BIN@\|@SYSMON_CONFIG_FILE@' "$(CURDIR)/.tmp/sysmon/etc/systemd/system/sysmon.service"; then \
		printf '%s\n' "Rendered sysmon unit still contains unresolved placeholders" >&2; \
		exit 1; \
	fi

build: $(BINARY)
	$(DOCKER) build --tag $(IMAGE_TAG) .

$(BINARY):
	mkdir -p "$(BUILD_DIR)"
	CGO_ENABLED=0 $(GO) build -o "$(BINARY)" ./cmd/conmon

build-sysmon: $(SYSMON_BINARY)

$(SYSMON_BINARY):
	mkdir -p "$(BUILD_DIR)"
	CGO_ENABLED=0 $(GO) build -o "$(SYSMON_BINARY)" ./cmd/sysmon

preflight-install:
	$(call require_safe_rm_path,$(INSTALL_ROOT),INSTALL_ROOT)
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && ! command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		printf '%s\n' "systemd integration requested but $(SYSTEMCTL) is unavailable. Install systemctl or rerun with ENABLE_SYSTEMD=0." >&2; \
		exit 1; \
	fi

install: preflight-install build
	mkdir -p "$(INSTALL_ROOT)" "$(CONFIG_DIR)" "$(DATA_DIR)/prometheus" "$(DATA_DIR)/grafana" "$(BIN_DIR)" "$(SYSTEMD_DIR)"
	if [ "$$(id -u)" = "0" ]; then \
		chown -R "$(PROMETHEUS_UID):$(PROMETHEUS_GID)" "$(DATA_DIR)/prometheus"; \
		chown -R "$(GRAFANA_UID):$(GRAFANA_GID)" "$(DATA_DIR)/grafana"; \
	fi
	for path in Dockerfile go.mod go.sum README.md cmd internal config deploy scripts; do \
		rm -rf "$(INSTALL_ROOT)/$$path"; \
	done
	tar -C . -cf - $(APP_TREE_ITEMS) | tar -C "$(INSTALL_ROOT)" -xf -
	$(INSTALL) -m 0755 "$(BINARY)" "$(HELPER_BIN)"
	$(INSTALL) -m 0755 scripts/conmon-grafana-admin "$(GRAFANA_ADMIN_HELPER)"
	if [ ! -e "$(CONFIG_FILE)" ]; then \
		$(INSTALL) -m 0644 config/conmon.example.yml "$(CONFIG_FILE)"; \
	else \
		printf '%s\n' "Preserving existing config at $(CONFIG_FILE)"; \
	fi
	unit_tmp="$$(mktemp "$(UNIT_FILE).tmp.XXXXXX")"; \
	trap 'rm -f "$$unit_tmp"' EXIT; \
	sed \
		-e 's|@INSTALL_ROOT@|$(INSTALL_ROOT)|g' \
		-e 's|@CONFIG_FILE@|$(CONFIG_FILE)|g' \
		-e 's|@DATA_DIR@|$(DATA_DIR)|g' \
		-e 's|@IMAGE_TAG@|$(IMAGE_TAG)|g' \
		deploy/systemd/conmon.service > "$$unit_tmp"; \
	chmod 0644 "$$unit_tmp"; \
	mv "$$unit_tmp" "$(UNIT_FILE)"; \
	trap - EXIT
	if [ "$(ENABLE_SYSTEMD)" = "1" ]; then \
		$(SYSTEMCTL) daemon-reload; \
		if [ "$(START_SERVICE)" = "1" ]; then \
			$(SYSTEMCTL) enable --now conmon.service; \
		else \
			printf '%s\n' "Installed $(UNIT_FILE); systemd enable/start skipped because START_SERVICE=$(START_SERVICE)"; \
		fi; \
	else \
		printf '%s\n' "Installed $(UNIT_FILE); systemd actions skipped because ENABLE_SYSTEMD=$(ENABLE_SYSTEMD)"; \
	fi

preflight-install-sysmon:
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && ! command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		printf '%s\n' "systemd integration requested but $(SYSTEMCTL) is unavailable. Install systemctl or rerun with ENABLE_SYSTEMD=0." >&2; \
		exit 1; \
	fi

install-sysmon: preflight-install-sysmon build-sysmon
	mkdir -p "$(SYSMON_CONFIG_DIR)" "$(SYSMON_BIN_DIR)" "$(SYSMON_SYSTEMD_DIR)"
	$(INSTALL) -m 0755 "$(SYSMON_BINARY)" "$(SYSMON_HELPER_BIN)"
	if [ ! -e "$(SYSMON_CONFIG_FILE)" ]; then \
		$(INSTALL) -m 0644 config/sysmon.example.yml "$(SYSMON_CONFIG_FILE)"; \
	else \
		printf '%s\n' "Preserving existing config at $(SYSMON_CONFIG_FILE)"; \
	fi
	unit_tmp="$$(mktemp "$(SYSMON_UNIT_FILE).tmp.XXXXXX")"; \
	trap 'rm -f "$$unit_tmp"' EXIT; \
	sed \
		-e 's|@SYSMON_HELPER_BIN@|$(SYSMON_HELPER_BIN)|g' \
		-e 's|@SYSMON_CONFIG_FILE@|$(SYSMON_CONFIG_FILE)|g' \
		deploy/systemd/sysmon.service > "$$unit_tmp"; \
	chmod 0644 "$$unit_tmp"; \
	mv "$$unit_tmp" "$(SYSMON_UNIT_FILE)"; \
	trap - EXIT
	if [ "$(ENABLE_SYSTEMD)" = "1" ]; then \
		$(SYSTEMCTL) daemon-reload; \
		if [ "$(START_SERVICE)" = "1" ]; then \
			$(SYSTEMCTL) enable --now sysmon.service; \
		else \
			printf '%s\n' "Installed $(SYSMON_UNIT_FILE); systemd enable/start skipped because START_SERVICE=$(START_SERVICE)"; \
		fi; \
	else \
		printf '%s\n' "Installed $(SYSMON_UNIT_FILE); systemd actions skipped because ENABLE_SYSTEMD=$(ENABLE_SYSTEMD)"; \
	fi

preflight-uninstall:
	$(call require_safe_rm_path,$(INSTALL_ROOT),INSTALL_ROOT)
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && ! command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		printf '%s\n' "systemd integration requested but $(SYSTEMCTL) is unavailable. Install systemctl or rerun with ENABLE_SYSTEMD=0." >&2; \
		exit 1; \
	fi

uninstall: preflight-uninstall
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		$(SYSTEMCTL) disable --now conmon.service >/dev/null 2>&1 || true; \
	fi
	rm -f "$(UNIT_FILE)"
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		$(SYSTEMCTL) daemon-reload || true; \
		$(SYSTEMCTL) reset-failed conmon.service >/dev/null 2>&1 || true; \
	fi
	rm -rf "$(INSTALL_ROOT)"
	rm -f "$(HELPER_BIN)"
	rm -f "$(GRAFANA_ADMIN_HELPER)"
	printf '%s\n' "Preserved $(CONFIG_FILE) and $(DATA_DIR)"

preflight-uninstall-sysmon:
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && ! command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		printf '%s\n' "systemd integration requested but $(SYSTEMCTL) is unavailable. Install systemctl or rerun with ENABLE_SYSTEMD=0." >&2; \
		exit 1; \
	fi

uninstall-sysmon: preflight-uninstall-sysmon
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		$(SYSTEMCTL) disable --now sysmon.service >/dev/null 2>&1 || true; \
	fi
	rm -f "$(SYSMON_UNIT_FILE)"
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		$(SYSTEMCTL) daemon-reload || true; \
		$(SYSTEMCTL) reset-failed sysmon.service >/dev/null 2>&1 || true; \
	fi
	rm -f "$(SYSMON_HELPER_BIN)"
	printf '%s\n' "Preserved $(SYSMON_CONFIG_FILE)"

preflight-clean:
	$(call require_safe_rm_path,$(BUILD_DIR),BUILD_DIR)

clean: preflight-clean
	rm -rf "$(BUILD_DIR)"
	if command -v "$(DOCKER)" >/dev/null 2>&1 && $(DOCKER) image inspect "$(IMAGE_TAG)" >/dev/null 2>&1; then \
		$(DOCKER) image rm -f "$(IMAGE_TAG)"; \
	fi
