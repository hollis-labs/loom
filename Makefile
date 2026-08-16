.PHONY: all build run install ui-build ui-dev test vet lint clean

# Build the frontend then the Go binary — a production binary with the
# Sysop UI embedded.
all: ui-build build

# Build the Sysop UI into internal/webui/dist (the Go embed directory).
ui-build:
	cd frontend && npm install && npm run build

# Run the Sysop UI dev server with hot reload. Proxies /api to the Go
# server on :8080 — run `make run` in another shell. The dev UI is served
# under // (see `base` in frontend/vite.config.ts).
ui-dev:
	cd frontend && npm install && npm run dev

# Build the Go binary. Embeds whatever is in internal/webui/dist; run
# `make ui-build` first for a binary that serves the real UI.
build:
	go build -o loom ./cmd/loom

# Build and run the server.
run: build
	./loom

# Build the embedded UI and install the binary to $GOBIN — a self-contained
# loom on PATH that serves the real Sysop UI.
install: ui-build
	go install ./cmd/loom

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; else echo "golangci-lint not installed; skipped"; fi
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; else echo "staticcheck not installed; skipped"; fi
	@if command -v errcheck >/dev/null 2>&1; then errcheck -ignoretests ./...; else echo "errcheck not installed; skipped"; fi
	@if command -v govulncheck >/dev/null 2>&1; then govulncheck ./...; else echo "govulncheck not installed; skipped"; fi

# Remove build artifacts. Keeps internal/webui/dist/.gitkeep so the
# //go:embed directive still compiles.
clean:
	rm -f loom
	rm -rf frontend/node_modules
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
