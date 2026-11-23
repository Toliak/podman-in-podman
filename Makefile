GO ?= go
BIN_DIR ?= bin
PY ?= .venv/bin/python
RUFF ?= .venv/bin/ruff

.PHONY: all build test lint vet clean selinux-test

all: build test lint

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/pinp-e ./cmd/pinp-e
	$(GO) build -o $(BIN_DIR)/pinp-i ./cmd/pinp-i

test: build
	$(PY) -m pytest -q

lint:
	$(RUFF) check .
	$(RUFF) format --check .
	$(GO) vet ./...

vet:
	$(GO) vet ./...

clean:
	rm -rf $(BIN_DIR)

selinux-test: build
	bash tests/selinux/run-matrix.sh
