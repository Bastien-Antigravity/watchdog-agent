#!/bin/bash

# Start NATS Server with JetStream
# This script starts the NATS server using the configuration file

NATS_CONFIG="nats-server.conf"

echo "Starting NATS server with JetStream..."

# Check if NATS server is installed
if ! command -v ./nats-server &> /dev/null; then
    echo "Error: nats-server is not installed"
    echo "Install it with binary from https://github.com/nats-io/nats-server/releases"
    exit 1
fi

# Check if config file exists
if [ ! -f "$NATS_CONFIG" ]; then
    echo "Error: Configuration file not found: $NATS_CONFIG"
    exit 1
fi

# Start NATS server
echo "Using config: $NATS_CONFIG"
./nats-server -c "$NATS_CONFIG"
