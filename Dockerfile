# === BUILD STAGE ===
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev ca-certificates tzdata

WORKDIR /workspace

# Copy required local modules for 'replace' directives
COPY microservice-toolbox ./microservice-toolbox
COPY universal-logger ./universal-logger
COPY safe-socket ./safe-socket

# Copy the target service
COPY watchdog-agent ./watchdog-agent

WORKDIR /workspace/watchdog-agent

# Ensure dependencies are tidy for linux build
RUN go mod tidy && go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /watchdog-agent-bin ./main.go

# === RUNTIME STAGE ===
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /watchdog-agent

# Copy the binary from the build stage
COPY --from=builder /watchdog-agent-bin /watchdog-agent/watchdog-agent

# Set the entrypoint
ENTRYPOINT ["/watchdog-agent/watchdog-agent"]
