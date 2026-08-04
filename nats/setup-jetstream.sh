#!/bin/bash

# NATS JetStream Setup Script
# This script creates the MARKET_DATA stream for the trading system

echo "Setting up NATS JetStream..."

# Check if NATS CLI is installed
if ! command -v ./nats-server &> /dev/null; then
    echo "Error: NATS CLI is not installed"
    echo "Install it with: brew install nats-io/nats-tools/nats"
    exit 1
fi

# Create the MARKET_DATA stream
echo "Creating MARKET_DATA stream..."
./nats-server stream add MARKET_DATA \
  --subjects "marketdata.>" \
  --storage file \
  --retention limits \
  --max-age 72h \
  --max-bytes 10GB \
  --max-msg-size 1MB \
  --replicas 1 \
  --defaults

if [ $? -eq 0 ]; then
    echo "✅ MARKET_DATA stream created successfully"
else
    echo "❌ Failed to create stream"
    exit 1
fi

# Verify the stream was created
echo ""
echo "Stream info:"
./nats-server stream info MARKET_DATA

echo ""
echo "✅ Setup complete! You can now start the trading system."
