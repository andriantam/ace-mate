.PHONY: web build run dev test lint fmt docker

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(shell go list -m)/internal/version.Version=$(VERSION)

web:            ## build the React UI into web/dist (embedded into the Go binary)
	cd web && npm ci && npm run build

build: web      ## build a single self-contained binary at bin/server
	go build -ldflags "$(LDFLAGS)" -o bin/server ./cmd/server

run: build      ## build and run
	./bin/server

dev:            ## API on :3000 (run `cd web && npm run dev` in another terminal for hot reload on :5173)
	go run ./cmd/server

test:           ## Go tests (security, queue, engine, providers), then UI lint, typecheck and tests
	go vet ./...
	go test ./...
	cd web && npm run lint && npm run typecheck && npm test

lint:           ## gofmt check for Go, ESLint for the UI
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	cd web && npm run lint

fmt:
	gofmt -w .

docker:         ## build the container image
	docker build --build-arg VERSION=$(VERSION) -t acemate:$(VERSION) .
