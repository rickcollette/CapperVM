APP         := capper
BIN_DIR     := bin
DIST_DIR    := DIST
DIST_STORE  := $(DIST_DIR)/store
DIST_APP    := $(DIST_DIR)/$(APP)
RUN_DIR     := capper-run
ALPINE_CONFIG := examples/alpine/capper.json
ALPINE_ROOTFS := examples/alpine/rootfs/bin/sh
CAPPERWEB_DIR ?= ../CapperWeb

.PHONY: build test clean distclean dist web bootstrap-alpine \
        capper-run capper-run-stop capper-run-status \
        setcap setup capdb capdb-fetch capdb-clean test-capdb build-capdb \
        docs docs-md docs-web docs-pdf docs-serve docs-check docs-clean docs-inventory \
        docs-gen docs-cli docs-api docs-screenshots git-push

# CapDB lives in its own repository (https://github.com/rickcollette/CapDB). It is
# checked out into ./CapDB (git-ignored) by `make capdb-fetch`, which clones or
# fast-forwards it from CAPDB_REPO. Override CAPDB_DIR to use a different checkout;
# CAPDB_BUILD stays inside Capper so the source tree is never written to.
CAPDB_REPO  ?= https://github.com/rickcollette/CapDB.git
CAPDB_DIR   ?= CapDB
CAPDB_BUILD ?= build/capdb

# ── Version stamping ──────────────────────────────────────────────────────────
# Read from the VERSION file; git metadata when available. Override CAPPER_VERSION
# to stamp a release build. These feed -ldflags -X into internal/version.
CAPPER_VERSION ?= $(shell cat VERSION 2>/dev/null || echo 0.0.0-dev)
CAPPER_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
CAPPER_DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG    := capper/internal/version
# NB: named GO_LDFLAGS, not LDFLAGS — the latter is the C linker's env var and is
# re-exported by make into sub-builds (e.g. CapDB's CMake), where these Go '-X'
# flags would break the C compiler check.
GO_LDFLAGS     := -X $(VERSION_PKG).Version=$(CAPPER_VERSION) \
                  -X $(VERSION_PKG).Commit=$(CAPPER_COMMIT) \
                  -X $(VERSION_PKG).BuildDate=$(CAPPER_DATE)

# cgo needs the client header + static lib, whose locations depend on CAPDB_DIR /
# CAPDB_BUILD. driver.go keeps only the portable flags; these supply the paths.
CAPDB_CGO_CFLAGS  := -I$(abspath $(CAPDB_DIR))/capdb/client
CAPDB_CGO_LDFLAGS := $(abspath $(CAPDB_BUILD))/libcapdb_client.a
CAPDB_SERVER_BIN  := $(abspath $(CAPDB_BUILD))/capdb-server

# ── Development entry point ───────────────────────────────────────────────────

# Full first-time or clean setup: build everything, grant capabilities, start.
setup:
	@CAPPERWEB_DIR="$(CAPPERWEB_DIR)" RUN_DIR="$(RUN_DIR)" scripts/setup.sh

# Re-apply capabilities after a rebuild (the only sudo step).
setcap:
	sudo scripts/capper-setcap.sh $(RUN_DIR)/lib/$(APP)-bin

# ── Individual build steps ────────────────────────────────────────────────────

build:
	mkdir -p $(BIN_DIR)
	go build -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/$(APP) ./cmd/capper
	go build -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/$(APP)-agent ./cmd/capper-agent

# Build the control-plane binary with the CapDB networked backend linked in
# (cgo + OpenSSL). This is the artifact to deploy when CapDB is the primary DB
# service (CAPPER_DB_DRIVER=capdb). The default `build` stays pure-Go.
build-capdb: capdb
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 \
	  CGO_CFLAGS="$(CAPDB_CGO_CFLAGS)" \
	  CGO_LDFLAGS="$(CAPDB_CGO_LDFLAGS)" \
	  go build -tags capdb -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/$(APP) ./cmd/capper

test:
	go test ./...

# ── CapDB networked storage backend (external tree at $(CAPDB_DIR)) ────────────

# Build the CapDB client library + server (CMake, requires a C toolchain +
# OpenSSL). Products land in $(CAPDB_BUILD): libcapdb_client.a, capdb-server.
# Always runs the (incremental) CMake build so edits to the CapDB sources are
# picked up; configure only runs the first time.
# Clone the CapDB repo into CAPDB_DIR (or fast-forward an existing checkout).
capdb-fetch:
	@if [ -d "$(CAPDB_DIR)/.git" ]; then \
	  echo "Updating CapDB checkout in $(CAPDB_DIR)"; git -C "$(CAPDB_DIR)" pull --ff-only; \
	else \
	  echo "Cloning $(CAPDB_REPO) -> $(CAPDB_DIR)"; git clone "$(CAPDB_REPO)" "$(CAPDB_DIR)"; \
	fi

capdb:
	@test -d "$(CAPDB_DIR)/capdb/client" || { \
	  echo "CapDB source not found at $(CAPDB_DIR); run 'make capdb-fetch' (or set CAPDB_DIR)"; exit 1; }
	@test -f $(CAPDB_BUILD)/CMakeCache.txt || \
	  cmake -B $(CAPDB_BUILD) -S $(CAPDB_DIR) -DCAPDB_ENABLE_POOL=ON -DCAPDB_ENABLE_NETWORK=ON
	cmake --build $(CAPDB_BUILD) -j$(shell nproc) --target capdb_client capdb-server capdbtest

capdb-clean:
	rm -rf $(CAPDB_BUILD)

# Run the cgo driver's integration suite against a real capdb-server.
# Requires `make capdb` first; uses the `capdb` build tag.
test-capdb: capdb
	CGO_CFLAGS="$(CAPDB_CGO_CFLAGS)" \
	  CGO_LDFLAGS="$(CAPDB_CGO_LDFLAGS)" \
	  CAPDB_SERVER="$(CAPDB_SERVER_BIN)" \
	  go test -tags capdb ./internal/capdbdriver/...

bootstrap-alpine: $(ALPINE_ROOTFS)

$(ALPINE_ROOTFS): examples/alpine/bootstrap.sh
	sh examples/alpine/bootstrap.sh

dist: build bootstrap-alpine
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR) $(DIST_STORE) $(DIST_DIR)/lib
	cp $(BIN_DIR)/$(APP) $(DIST_DIR)/lib/$(APP)-bin
	cp scripts/dist-capper-wrapper.sh $(DIST_APP)
	chmod +x $(DIST_APP)
	cp README.md go.mod go.sum $(DIST_DIR)/
	cp -a schemas $(DIST_DIR)/
	cp -a docs $(DIST_DIR)/
	cp -a examples $(DIST_DIR)/
	cd $(DIST_DIR) && ./$(APP) --store ./store create alpine.cap examples/alpine/capper.json
	cp $(DIST_STORE)/images/alpine.cap $(DIST_DIR)/alpine.cap

web:
	cd $(CAPPERWEB_DIR) && VITE_CAPPER_VERSION=$(CAPPER_VERSION) scripts/build.sh

# ── Service management ────────────────────────────────────────────────────────

capper-run: dist web
	mkdir -p $(DIST_DIR)/console
	cp -a $(CAPPERWEB_DIR)/dist/. $(DIST_DIR)/console/
	RUN_DIR="$(RUN_DIR)" scripts/capper-run.sh start

capper-run-stop:
	RUN_DIR="$(RUN_DIR)" scripts/capper-run.sh stop

capper-run-status:
	RUN_DIR="$(RUN_DIR)" scripts/capper-run.sh status

# ── Housekeeping ──────────────────────────────────────────────────────────────

# Remove every build/runtime artifact (everything git-ignored that we generate).
# Keeps the CapDB source checkout — use `distclean` to remove that too.
clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) build
	rm -f $(APP) $(APP)-agent capinit docgen
	rm -rf docs/dist docs/generated
	rm -rf $(RUN_DIR)/store $(RUN_DIR)/logs $(RUN_DIR)/run $(RUN_DIR)/lib \
	       $(RUN_DIR)/console $(RUN_DIR)/capper
	rm -rf examples/alpine/downloads examples/alpine/rootfs
	find . -path ./CapDB -prune -o -type f \
	  \( -name '*.test' -o -name '*.out' -o -name '*.prof' -o -name '.DS_Store' \) \
	  -print -delete 2>/dev/null || true

# Full clean: also remove the fetched CapDB source checkout (re-fetch with
# `make capdb-fetch`).
distclean: clean
	rm -rf $(CAPDB_DIR)

# ── Documentation ─────────────────────────────────────────────────────────────

docs: docs-gen docs-check docs-inventory docs-md docs-web docs-pdf

# Regenerate the source-derived reference (CLI command tree + API routes) before
# checking/building, so the reference always matches the code.
docs-gen: docs-cli docs-api

docs-cli:
	go run ./tools/docgen clidocs

docs-api:
	go run ./tools/docgen apidocs

docs-check:
	go run ./tools/docgen check

docs-inventory:
	go run ./tools/docgen inventory

docs-md:
	go run ./tools/docgen markdown

docs-web:
	go run ./tools/docgen web

docs-screenshots:
	scripts/docs-screenshots.sh

docs-pdf:
	go run ./tools/docgen pdf

docs-serve:
	go run ./tools/docgen serve

docs-clean:
	rm -rf docs/dist

# ── Push helper ───────────────────────────────────────────────────────────────

# Regenerate the source-derived CLI/API reference, commit it if it changed, then
# push. This keeps the CI "docs" check green (it fails when capper.md/routes.md
# are out of date relative to the code).
git-push: docs-gen
	@git add docs/src/reference/cli/capper.md docs/src/reference/api/routes.md
	@if git diff --cached --quiet -- docs/src/reference; then \
	  echo "docs reference already up to date"; \
	else \
	  git commit -m "docs: regenerate CLI/API reference"; \
	fi
	git push
