.DEFAULT_GOAL := help

.PHONY: help fmt test backend frontend-check frontend-build browser-build desktop-build build-browser build-desktop build clean run

BACKEND_DIR := src/backend
FRONTEND_DIR := src/frontend/frontend
DESKTOP_DIR := src/frontend
DIST_DIR := dist

GO_TAGS ?= sqlite_fts5
WAILS_TAGS ?= webkit2_41
GO_ENV := CGO_ENABLED=1


# Help

help: ## Display available commands
	@echo "Scriptorium build system"
	@echo ""
	@echo "Usage: make <command>"
	@echo ""
	@echo "Available commands:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Development and quality checks

fmt: ## Format all Go source files
	find src/backend -name '*.go' -print0 | xargs -0 gofmt -w

test: ## Run all backend Go tests
	cd $(BACKEND_DIR) && $(GO_ENV) go test -tags "$(GO_TAGS)" ./...

frontend-check: ## Install frontend dependencies and run Svelte checks
	cd $(FRONTEND_DIR) && npm ci && npm run check

# Individual build components

backend: ## Build the standalone Go backend into dist/backend
	mkdir -p $(DIST_DIR)/backend
	cd $(BACKEND_DIR) && $(GO_ENV) go build \
		-tags "$(GO_TAGS)" \
		-trimpath \
		-ldflags="-s -w" \
		-o ../../$(DIST_DIR)/backend/scriptorium \
		.

frontend-build: ## Build the browser frontend into dist/web
	cd $(FRONTEND_DIR) && npm ci && npm run build
	mkdir -p $(DIST_DIR)/web
	cp -a $(FRONTEND_DIR)/dist/. $(DIST_DIR)/web/

# Complete browser/server distribution

browser-build: frontend-build ## Build the browser frontend and backend
	rm -rf $(BACKEND_DIR)/web
	mkdir -p $(BACKEND_DIR)/web
	cp -a $(DIST_DIR)/web/. $(BACKEND_DIR)/web/
	$(MAKE) backend

build-browser: browser-build ## Package a complete browser/server distribution
	mkdir -p $(DIST_DIR)/browser
	cp $(DIST_DIR)/backend/scriptorium $(DIST_DIR)/browser/
	cp -a $(DIST_DIR)/web $(DIST_DIR)/browser/

# Complete desktop application

desktop-build: ## Build the complete Wails desktop application
	cd $(DESKTOP_DIR) && wails build -tags "$(WAILS_TAGS)"
	mkdir -p $(DIST_DIR)/desktop
	cp -a $(DESKTOP_DIR)/build/bin/. $(DIST_DIR)/desktop/

build-desktop: desktop-build ## Build and package the desktop application

# Build all distributable targets

build: build-browser build-desktop ## Build all application distributions

# Development

run: ## Run the backend directly using Go
	cd $(BACKEND_DIR) && $(GO_ENV) go run -tags "$(GO_TAGS)" .

# Cleanup

clean: ## Remove all generated build artifacts
	rm -rf $(DIST_DIR)
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(BACKEND_DIR)/web
	rm -rf $(DESKTOP_DIR)/build/bin
