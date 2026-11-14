.PHONY: fmt test backend frontend-check browser-build desktop-build run

BACKEND_DIR := src/backend
FRONTEND_DIR := src/frontend/frontend

fmt:
	find src/backend -name '*.go' -print0 | xargs -0 gofmt -w

test:
	cd $(BACKEND_DIR) && go test ./...

backend:
	cd $(BACKEND_DIR) && go build ./...

frontend-check:
	cd $(FRONTEND_DIR) && npm install && npm run check && npm run build

browser-build:
	cd $(FRONTEND_DIR) && npm install && npm run build
	mkdir -p $(BACKEND_DIR)/web
	cp -a $(FRONTEND_DIR)/dist/. $(BACKEND_DIR)/web/

desktop-build:
	cd src/frontend && wails build

run:
	cd $(BACKEND_DIR) && go run .
