#!/bin/bash
# Simple benchmark script without color codes and bc dependency

set -e

cd "$(dirname "$0")"

echo "=== Demo Data Generator Benchmark ==="
echo ""

# Check services
if ! nc -z localhost 3000 >/dev/null 2>&1; then
  echo "Error: Onboarding service not running on port 3000"
  exit 1
fi

if ! nc -z localhost 3001 >/dev/null 2>&1; then
  echo "Error: Transaction service not running on port 3001"
  exit 1
fi

echo "Services are running"
echo ""

# Run standard generator
echo "Running standard generator..."
start_time=$(date +%s)
if ./run-generator.sh small none > benchmark_standard.log 2>&1; then
  end_time=$(date +%s)
  standard_time=$((end_time - start_time))
  echo "Standard generator completed in ${standard_time} seconds"
  
  # Extract stats
  grep -E "(Organizations|Ledgers|Accounts|Transactions):" benchmark_standard.log | tail -4
else
  echo "Standard generator failed"
  exit 1
fi

echo ""

# Run optimized generator
echo "Running optimized generator..."
start_time=$(date +%s)
if ./run-generator.sh small none --optimized > benchmark_optimized.log 2>&1; then
  end_time=$(date +%s)
  optimized_time=$((end_time - start_time))
  echo "Optimized generator completed in ${optimized_time} seconds"
  
  # Extract stats
  grep -E "(Organizations|Ledgers|Accounts|Transactions):" benchmark_optimized.log | tail -4
else
  echo "Optimized generator failed"
  exit 1
fi

echo ""
echo "=== Performance Summary ==="
echo "Standard generator: ${standard_time} seconds"
echo "Optimized generator: ${optimized_time} seconds"

if [ "$optimized_time" -gt 0 ]; then
  improvement=$(( (standard_time - optimized_time) * 100 / standard_time ))
  echo "Improvement: ${improvement}% faster"
fi