SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

GO ?= go
DOCKER ?= docker
INSTALL ?= install
SYSTEMCTL ?= systemctl

BUILD_DIR ?= build
BINARY := $(BUILD_DIR)/conmon
IMAGE_TAG ?= conmon:local

INSTALL_ROOT ?= /opt/conmon
CONFIG_DIR ?= /etc/conmon
CONFIG_FILE ?= $(CONFIG_DIR)/config.yml
DATA_DIR ?= /var/lib/conmon
BIN_DIR ?= /usr/local/bin
HELPER_BIN ?= $(BIN_DIR)/conmon
SYSTEMD_DIR ?= /etc/systemd/system
UNIT_FILE ?= $(SYSTEMD_DIR)/conmon.service

ENABLE_SYSTEMD ?= 1
START_SERVICE ?= 1

APP_TREE_ITEMS := Dockerfile go.mod go.sum cmd internal config deploy README.md

define require_safe_rm_path
case "$(strip $(1))" in ""|"/"|"."|"..") printf '%s\n' "Refusing destructive operation with $(2)=$(strip $(1)). Choose a non-empty path that is not /, . or .." >&2; exit 1;; esac
endef

.PHONY: build install uninstall clean preflight-install preflight-uninstall preflight-clean

build: $(BINARY)
	$(DOCKER) build --tag $(IMAGE_TAG) .

$(BINARY):
	mkdir -p "$(BUILD_DIR)"
	CGO_ENABLED=0 $(GO) build -o "$(BINARY)" ./cmd/conmon

preflight-install:
	$(call require_safe_rm_path,$(INSTALL_ROOT),INSTALL_ROOT)
	if [ "$(ENABLE_SYSTEMD)" = "1" ] && ! command -v "$(SYSTEMCTL)" >/dev/null 2>&1; then \
		printf '%s\n' "systemd integration requested but $(SYSTEMCTL) is unavailable. Install systemctl or rerun with ENABLE_SYSTEMD=0." >&2; \
		exit 1; \
	fi

install: preflight-install build
	mkdir -p "$(INSTALL_ROOT)" "$(CONFIG_DIR)" "$(DATA_DIR)/prometheus" "$(DATA_DIR)/grafana" "$(BIN_DIR)" "$(SYSTEMD_DIR)"
	for path in Dockerfile go.mod go.sum README.md cmd internal config deploy; do \
		rm -rf "$(INSTALL_ROOT)/$$path"; \
	done
	tar -C . -cf - $(APP_TREE_ITEMS) | tar -C "$(INSTALL_ROOT)" -xf -
	$(INSTALL) -m 0755 "$(BINARY)" "$(HELPER_BIN)"
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
	printf '%s\n' "Preserved $(CONFIG_FILE) and $(DATA_DIR)"

preflight-clean:
	$(call require_safe_rm_path,$(BUILD_DIR),BUILD_DIR)

clean: preflight-clean
	rm -rf "$(BUILD_DIR)"
	if command -v "$(DOCKER)" >/dev/null 2>&1 && $(DOCKER) image inspect "$(IMAGE_TAG)" >/dev/null 2>&1; then \
		$(DOCKER) image rm -f "$(IMAGE_TAG)"; \
	fi
