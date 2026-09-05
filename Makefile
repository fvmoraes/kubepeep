SHELL := /bin/sh

APP := kubePeep
GO ?= go
NPM ?= npm
GINGER ?= $(shell $(GO) env GOPATH)/bin/ginger
WEB_DIR := web
DIST_DIR := dist
BINARY := $(DIST_DIR)/$(APP)
GO_FILES := $(shell find cmd internal -type f -name '*.go' 2>/dev/null)
GO_PACKAGES := $(shell $(GO) list ./... 2>/dev/null | grep -v '/web/node_modules/')
VERSION ?= 0.1.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/fvmoraes/kubepeep/internal/buildinfo.Version=$(VERSION) \
	-X github.com/fvmoraes/kubepeep/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/fvmoraes/kubepeep/internal/buildinfo.BuildDate=$(BUILD_DATE)

.PHONY: format format-check lint typecheck test-unit test-integration test-race \
	test-e2e test web-install web-build build smoke cross-build verify-ginger \
	verify clean dev-desktop build-desktop build-desktop-linux \
	build-desktop-windows build-desktop-darwin

WAILS ?= $(shell $(GO) env GOPATH)/bin/wails
# WebKitGTK: prefer 4.0 (upstream default); fall back to 4.1 via Wails'
# webkit2_41 build tag where only the newer package is installed.
WEBKIT41 := $(shell pkg-config --exists webkit2gtk-4.0 2>/dev/null || echo yes)
ifeq ($(WEBKIT41),yes)
DESKTOP_TAGS := desktop webkit2_41
else
DESKTOP_TAGS := desktop
endif
DESKTOP_OUT := $(DIST_DIR)/desktop
# Keep the reviewed Bridge contract, as the native release workflows do.
WAILS_BINDING_FLAGS := -skipbindings

format:
	gofmt -w $(GO_FILES)
	cd $(WEB_DIR) && $(NPM) run format

format-check:
	@test -z "$$(gofmt -l $(GO_FILES))"
	cd $(WEB_DIR) && $(NPM) run format:check

lint:
	$(GO) vet $(GO_PACKAGES)
	cd $(WEB_DIR) && $(NPM) run lint

typecheck:
	cd $(WEB_DIR) && $(NPM) run typecheck

test-unit: web-build
	$(GO) test $(GO_PACKAGES)
	cd $(WEB_DIR) && $(NPM) test

test-integration: web-build
	$(GO) test $(GO_PACKAGES) -run Integration

test-race: web-build
	CGO_ENABLED=1 $(GO) test -race $(GO_PACKAGES)

test-e2e: web-build
	cd $(WEB_DIR) && $(NPM) run test:e2e

test: test-unit test-integration

web-install:
	cd $(WEB_DIR) && $(NPM) ci

web-build: web-install
	cd $(WEB_DIR) && $(NPM) run build

build: web-build
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/kubePeep

smoke: build
	./scripts/smoke.sh $(BINARY)

cross-build: web-build
	@set -eu; \
	for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do \
		goos=$${target%/*}; goarch=$${target#*/}; suffix=""; \
		if [ "$$goos" = windows ]; then suffix=.exe; fi; \
		mkdir -p "$(DIST_DIR)/$$goos-$$goarch"; \
		GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 $(GO) build -trimpath \
			-ldflags "$(LDFLAGS)" -o "$(DIST_DIR)/$$goos-$$goarch/$(APP)$$suffix" ./cmd/kubePeep; \
	done

verify-ginger:
	$(GINGER) inspect
	$(GINGER) doctor

# Desktop (Wails v2) targets. Linux builds require libgtk-3-dev and
# libwebkit2gtk-4.0-dev; macOS builds require Xcode command line tools; Windows
# builds require WebView2 (no CGO). Cross-platform releases are produced by the
# per-OS native CI runners.
dev-desktop:
	$(WAILS) dev $(WAILS_BINDING_FLAGS) -tags "$(DESKTOP_TAGS)"

build-desktop:
	$(WAILS) build $(WAILS_BINDING_FLAGS) -tags "$(DESKTOP_TAGS)" -clean -o "$(DESKTOP_OUT)/kubePeep"

build-desktop-linux:
	$(WAILS) build $(WAILS_BINDING_FLAGS) -tags "$(DESKTOP_TAGS)" -clean -platform linux/amd64 -o "$(DESKTOP_OUT)/linux-amd64/kubePeep"

build-desktop-windows:
	$(WAILS) build $(WAILS_BINDING_FLAGS) -tags "$(DESKTOP_TAGS)" -clean -platform windows/amd64 -o "$(DESKTOP_OUT)/windows-amd64/kubePeep.exe"

build-desktop-darwin:
	$(WAILS) build $(WAILS_BINDING_FLAGS) -tags "$(DESKTOP_TAGS)" -clean -platform darwin/amd64 -o "$(DESKTOP_OUT)/darwin-amd64/kubePeep"

verify: format-check lint typecheck test test-e2e build smoke verify-ginger

clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)/linux-* $(DIST_DIR)/darwin-* $(DIST_DIR)/windows-*
