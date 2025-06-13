#!/bin/bash
# Test process delay functionality

set -e

cd "$(dirname "$0")"

echo "=== Testing Process Delay Functionality ==="
echo ""

# Test with no delay (0 seconds)
echo "1. Testing with no delay (--process-delay 0)..."
start_time=$(date +%s)
./run-generator.sh small none --optimized --process-delay 0 > test-delay-0.log 2>&1
end_time=$(date +%s)
no_delay_time=$((end_time - start_time))
echo "   Completed in ${no_delay_time} seconds"

echo ""

# Test with 2 second delay
echo "2. Testing with 2 second delay (--process-delay 2)..."
start_time=$(date +%s)
./run-generator.sh small none --optimized --process-delay 2 > test-delay-2.log 2>&1
end_time=$(date +%s)
delay_time=$((end_time - start_time))
echo "   Completed in ${delay_time} seconds"

echo ""

# Check for delay messages in log
echo "3. Checking for delay messages in log..."
delay_count=$(grep -c "Waiting .* seconds for .* to propagate through RabbitMQ/Redis" test-delay-2.log || echo "0")
echo "   Found ${delay_count} delay messages"

echo ""
echo "=== Summary ==="
echo "No delay run: ${no_delay_time} seconds"
echo "With 2s delay: ${delay_time} seconds"
echo "Time difference: $((delay_time - no_delay_time)) seconds"
echo "Expected minimum difference: ~10 seconds (5 delays * 2 seconds)"

# Show delay messages
echo ""
echo "Delay messages from log:"
grep "Waiting .* seconds for .* to propagate through RabbitMQ/Redis" test-delay-2.log || echo "No delay messages found"