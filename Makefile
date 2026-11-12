.PHONY: all build run install ui-build ui-dev test vet lint pins scan generate clean

SUBSTRATE := go run github.com/hollis-labs/substrate/cmd/substrate

# Build the frontend then the Go binary — a production binary with the
# Sysop UI embedded.
all: ui-build build

# Build the Sysop UI into internal/webui/dist (the Go embed directory).
ui-build:
	cd frontend && npm ci && npm run build

# Run the Sysop UI dev server with hot reload. Proxies /api to the Go
# server (default 127.0.0.1:8080) — run `make run` in another shell. The dev UI is served
# at the root path (see `base` in frontend/vite.config.ts).
ui-dev:
	cd frontend && npm ci && npm run dev

# Build the Go binary. Embeds whatever is in internal/webui/dist; run
# `make ui-build` first for a binary that serves the real UI.
build:
	go build -o loom ./cmd/loom

# Build and run the server. Data lives in ./.loom (project-local), or where -db/-workspace/--no-project
# point; see docs/architecture.md.
run: build
	./loom

# Build the embedded UI and install the binary to $GOBIN — a self-contained
# loom on PATH that serves the real Sysop UI.
install: ui-build
	go install ./cmd/loom

test:
	go test -race ./...

vet:
	go vet ./...

lint: vet
	golangci-lint run

# The go.mod pins must match the Substrate BOM; edit the BOM (in Substrate), never go.mod.
pins:
	$(SUBSTRATE) pins check --app .

scan:
	# Tree only: the history gate returns after the program-wide history pass.
	python3 scripts/scan-public --tree

# Regenerate the wiring file after editing cmd/loom/substrate.yaml.
generate:
	$(SUBSTRATE) generate -f cmd/loom/substrate.yaml

# Remove build artifacts. Keeps internal/webui/dist/.gitkeep so the
# //go:embed directive still compiles.
clean:
	rm -f loom
	rm -rf frontend/node_modules
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
