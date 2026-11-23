GO ?= go
BIN_DIR ?= bin
PY ?= .venv/bin/python
RUFF ?= .venv/bin/ruff
PYTEST_CONFIG ?= tests/pyproject.toml

.PHONY: all build test lint vet clean selinux-test

all: build test lint

build:
	mkdir -p $(BIN_DIR)
	$(GO) -C pinp build -o ../$(BIN_DIR)/pinp-e ./cmd/pinp-e
	$(GO) -C pinp build -o ../$(BIN_DIR)/pinp-i ./cmd/pinp-i

test: build
	$(GO) -C pinp test ./...
	$(PY) -m pytest -q -c $(PYTEST_CONFIG) tests

lint:
	$(RUFF) check --config $(PYTEST_CONFIG) .
	$(RUFF) format --check --config $(PYTEST_CONFIG) .
	$(GO) -C pinp vet ./...

vet:
	$(GO) -C pinp vet ./...

clean:
	rm -rf $(BIN_DIR)

selinux-test: build
	PINP_SELINUX_TEST=1 $(PY) -m pytest -q -s -c $(PYTEST_CONFIG) -m selinux tests/selinux/test_matrix.py
