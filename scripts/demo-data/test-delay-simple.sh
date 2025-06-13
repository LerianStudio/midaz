#!/bin/bash
# Simple test to show process delay impact

set -e
cd "$(dirname "$0")"

echo "=== Process Delay Comparison ==="
echo ""

# Test with 0 second delay
echo "1. Running with 0 second delay..."
time ./run-generator.sh small none --optimized --process-delay 0 > /dev/null 2>&1
echo ""

# Test with default 5 second delay  
echo "2. Running with default 5 second delay..."
time ./run-generator.sh small none --optimized > /dev/null 2>&1
echo ""

echo "The difference shows the impact of process delays on total runtime."