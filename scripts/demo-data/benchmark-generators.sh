#!/bin/bash
# Benchmark script to compare standard vs optimized generator performance

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Go to script directory
cd "$(dirname "$0")"

echo -e "${BLUE}=== Demo Data Generator Benchmark ===${NC}"
echo ""

# Check if services are running
echo -e "${YELLOW}Checking if Midaz services are running...${NC}"
if ! nc -z localhost 3000 >/dev/null 2>&1; then
  echo -e "${RED}Error: Onboarding service not running on port 3000${NC}"
  echo "Please start Midaz services with 'make up' before running this benchmark"
  exit 1
fi

if ! nc -z localhost 3001 >/dev/null 2>&1; then
  echo -e "${RED}Error: Transaction service not running on port 3001${NC}"
  echo "Please start Midaz services with 'make up' before running this benchmark"
  exit 1
fi

echo -e "${GREEN}✓ Services are running${NC}"
echo ""

# Function to run a generator and measure time
run_generator() {
  local generator_type=$1
  local volume=$2
  local flags=$3
  
  echo -e "${YELLOW}Running ${generator_type} generator with volume: ${volume}...${NC}"
  
  # Clean previous run data by resetting database (optional - comment out if not wanted)
  # echo "Cleaning previous data..."
  # make reset-db > /dev/null 2>&1
  
  # Run generator and capture time
  local start_time=$(date +%s)
  
  if ./run-generator.sh "$volume" "none" $flags > benchmark_${generator_type}_${volume}.log 2>&1; then
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo -e "${GREEN}✓ ${generator_type} completed in ${duration} seconds${NC}"
    
    # Extract statistics from log - handle both formats
    local org_count=$(grep -E "Organizations:[[:space:]]+[0-9]+" benchmark_${generator_type}_${volume}.log | grep -oE "[0-9]+" | tail -1 || echo "0")
    local ledger_count=$(grep -E "Ledgers:[[:space:]]+[0-9]+" benchmark_${generator_type}_${volume}.log | grep -oE "[0-9]+" | tail -1 || echo "0")
    local account_count=$(grep -E "Accounts:[[:space:]]+[0-9]+" benchmark_${generator_type}_${volume}.log | grep -oE "[0-9]+" | tail -1 || echo "0")
    local tx_count=$(grep -E "Transactions:[[:space:]]+[0-9]+" benchmark_${generator_type}_${volume}.log | grep -oE "[0-9]+" | tail -1 || echo "0")
    local error_count=$(grep -E "Errors encountered:[[:space:]]+[0-9]+" benchmark_${generator_type}_${volume}.log | grep -oE "[0-9]+" | tail -1 || echo "0")
    
    echo "  - Organizations: $org_count"
    echo "  - Ledgers: $ledger_count"
    echo "  - Accounts: $account_count"
    echo "  - Transactions: $tx_count"
    if [ "$error_count" -gt 0 ]; then
      echo -e "  - ${RED}Errors: $error_count${NC}"
    fi
    
    echo "$duration"
  else
    echo -e "${RED}✗ ${generator_type} failed${NC}"
    echo "Check benchmark_${generator_type}_${volume}.log for details"
    echo "-1"
  fi
}

# Test with small volume first
echo -e "${BLUE}=== Testing with SMALL volume ===${NC}"
echo ""

# Run standard generator
standard_time=$(run_generator "standard" "small" "")
echo ""

# Run optimized generator
optimized_time=$(run_generator "optimized" "small" "--optimized")
echo ""

# Calculate improvement
if [ "$standard_time" != "-1" ] && [ "$optimized_time" != "-1" ]; then
  # Use integer arithmetic for percentage (multiply by 100 first to avoid losing precision)
  improvement=$(( (standard_time - optimized_time) * 100 / standard_time ))
  # Calculate speedup as a ratio (e.g., 2x, 3x)
  if [ "$optimized_time" -gt 0 ]; then
    speedup_whole=$((standard_time / optimized_time))
    speedup_remainder=$(( (standard_time * 10 / optimized_time) % 10 ))
    speedup="${speedup_whole}.${speedup_remainder}"
  else
    speedup="N/A"
  fi
  
  echo -e "${BLUE}=== Performance Summary ===${NC}"
  echo -e "Standard generator: ${standard_time}s"
  echo -e "Optimized generator: ${optimized_time}s"
  echo -e "${GREEN}Improvement: ${improvement}% faster${NC}"
  echo -e "${GREEN}Speedup: ${speedup}x${NC}"
fi

echo ""
echo "Log files saved:"
echo "  - benchmark_standard_small.log"
echo "  - benchmark_optimized_small.log"
echo ""

# Optionally test with larger volumes
read -p "Test with MEDIUM volume? (y/N): " test_medium
if [ "$test_medium" = "y" ] || [ "$test_medium" = "Y" ]; then
  echo ""
  echo -e "${BLUE}=== Testing with MEDIUM volume ===${NC}"
  echo ""
  
  standard_med=$(run_generator "standard" "medium" "")
  echo ""
  optimized_med=$(run_generator "optimized" "medium" "--optimized")
  echo ""
  
  if [ "$standard_med" != "-1" ] && [ "$optimized_med" != "-1" ]; then
    # Use integer arithmetic for percentage
    improvement_med=$(( (standard_med - optimized_med) * 100 / standard_med ))
    # Calculate speedup as a ratio
    if [ "$optimized_med" -gt 0 ]; then
      speedup_med_whole=$((standard_med / optimized_med))
      speedup_med_remainder=$(( (standard_med * 10 / optimized_med) % 10 ))
      speedup_med="${speedup_med_whole}.${speedup_med_remainder}"
    else
      speedup_med="N/A"
    fi
    
    echo -e "${BLUE}=== Medium Volume Performance ===${NC}"
    echo -e "Standard generator: ${standard_med}s"
    echo -e "Optimized generator: ${optimized_med}s"
    echo -e "${GREEN}Improvement: ${improvement_med}% faster${NC}"
    echo -e "${GREEN}Speedup: ${speedup_med}x${NC}"
  fi
fi

echo ""
echo -e "${GREEN}Benchmark completed!${NC}"