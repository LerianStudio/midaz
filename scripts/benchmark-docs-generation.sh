#!/bin/bash

echo "========================================"
echo "Documentation Generation Benchmark"
echo "========================================"
echo ""

# Function to run and time a command
benchmark() {
    local name=$1
    local script=$2
    
    echo "Running: $name"
    echo "----------------------------------------"
    
    # Clean up any existing swagger files to ensure fresh generation
    rm -f components/onboarding/api/swagger.json components/onboarding/api/openapi.yaml
    rm -f components/transaction/api/swagger.json components/transaction/api/openapi.yaml
    
    # Time the execution
    start_time=$(date +%s.%N)
    
    if $script > /tmp/benchmark.log 2>&1; then
        end_time=$(date +%s.%N)
        elapsed=$(echo "$end_time - $start_time" | bc)
        printf "✅ Success - Time: %.2f seconds\n" "$elapsed"
    else
        end_time=$(date +%s.%N)
        elapsed=$(echo "$end_time - $start_time" | bc)
        printf "❌ Failed - Time: %.2f seconds\n" "$elapsed"
        echo "Error output:"
        tail -20 /tmp/benchmark.log
    fi
    
    echo ""
}

# Run benchmarks
benchmark "Original Script" "./scripts/generate-docs.sh"
benchmark "Optimized Script" "./scripts/generate-docs-optimized.sh"

echo "========================================"
echo "Benchmark Complete"
echo "========================================"