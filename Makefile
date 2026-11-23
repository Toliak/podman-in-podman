GO ?= go
BIN_DIR ?= bin
UV ?= uv
UV_RUN ?= $(UV) run --project tests --no-sync
PYTEST_CONFIG ?= tests/pyproject.toml

.PHONY: all build test lint vet clean selinux-test sync

all: build test lint

build:
	mkdir -p $(BIN_DIR)
	$(GO) -C pinp build -o ../$(BIN_DIR)/pinp-e ./cmd/pinp-e
	$(GO) -C pinp build -o ../$(BIN_DIR)/pinp-i ./cmd/pinp-i

sync:
	$(UV) sync --project tests --frozen

test: build sync
	$(GO) -C pinp test ./...
	$(UV_RUN) pytest -q -c $(PYTEST_CONFIG) tests

lint: sync
	$(UV_RUN) ruff check --config $(PYTEST_CONFIG) .
	$(UV_RUN) ruff format --check --config $(PYTEST_CONFIG) .
	$(GO) -C pinp vet ./...

vet:
	$(GO) -C pinp vet ./...

clean:
	rm -rf $(BIN_DIR)

selinux-test: build sync
	PINP_SELINUX_TEST=1 $(UV_RUN) pytest -q -s -c $(PYTEST_CONFIG) -m selinux tests/selinux/test_matrix.py
