VERSION := $(strip $(shell cat VERSION.txt 2>/dev/null))
$(if $(VERSION),,$(error VERSION.txt is missing or empty))
LDFLAGS := -s -w -X 'github.com/Bastien-Antigravity/watchdog-agent/src/server.ServerVersion=$(VERSION)'

.PHONY: all build test verify-standalone version clean

all: build

version:
	@echo $(VERSION)

verify-standalone:
	@echo "Verifying standalone Go build (GOWORK=off)..."
	GOWORK=off go build ./...

build:
	@echo "Building watchdog-agent (version $(VERSION))..."
	@mkdir -p bin
	@if [ -f "go.mod" ]; then go build -ldflags="$(LDFLAGS)" -o bin/watchdog-agent ./main.go; fi

test:
	@echo "Running tests (version $(VERSION))..."
	@if [ -f "go.mod" ]; then go test -v ./...; fi

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/watchdog-agent dist build
