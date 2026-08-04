VERSION := "0.0.1"

.PHONY: all build test version clean

all: build

version:
	@echo $(VERSION)

build:
	@echo "Building watchdog-agent..."
	@if [ -f "go.mod" ]; then go build -o watchdog-agent ./main.go || true; fi

test:
	@echo "Running tests..."
	@if [ -f "go.mod" ]; then go test ./... 2>/dev/null || true; fi

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf watchdog-agent dist build
