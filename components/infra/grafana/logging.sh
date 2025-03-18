#!/bin/bash

# Helper function to log output from services
run_with_logging() {
  SERVICE_NAME=$1
  ENABLE_LOGS=$2
  shift 2
  
  if [ "$ENABLE_LOGS" = "true" ]; then
    echo "Starting $SERVICE_NAME with logging enabled"
    "$@" 2>&1 | while read -r line; do
      echo "[$SERVICE_NAME] $line"
    done
  else
    echo "Starting $SERVICE_NAME with logging disabled"
    "$@" > /dev/null 2>&1
  fi
} 