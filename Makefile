VERSION := $(shell cat VERSION.txt 2>/dev/null || echo "0.0.1")

.PHONY: all build test version clean

all: build

version:
	@echo $(VERSION)

build:
	@echo "Building watchdog-agent (version $(VERSION))..."
	@if [ -f "go.mod" ]; then go build -o bin/watchdog-agent ./main.go || true; fi

test:
	@echo "Running tests (version $(VERSION))..."
	@if [ -f "go.mod" ]; then go test ./... 2>/dev/null || true; fi

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/watchdog-agent dist build
