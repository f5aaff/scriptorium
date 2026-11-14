.PHONY: fmt test backend frontend-check browser-build desktop-build run

BACKEND_DIR := src/backend
FRONTEND_DIR := src/frontend/frontend
GO_TAGS ?= sqlite_fts5

fmt:
	find src/backend -name '*.go' -print0 | xargs -0 gofmt -w

test:
	cd $(BACKEND_DIR) && CGO_ENABLED=1 go test -tags "$(GO_TAGS)" ./...

backend:
	cd $(BACKEND_DIR) && CGO_ENABLED=1 go build -tags "$(GO_TAGS)" ./...

frontend-check:
	cd $(FRONTEND_DIR) && npm install && npm run check && npm run build

browser-build:
	cd $(FRONTEND_DIR) && npm install && npm run build
	mkdir -p $(BACKEND_DIR)/web
	cp -a $(FRONTEND_DIR)/dist/. $(BACKEND_DIR)/web/

desktop-build:
	cd src/frontend && wails build

run:
	cd $(BACKEND_DIR) && CGO_ENABLED=1 go run -tags "$(GO_TAGS)" .
