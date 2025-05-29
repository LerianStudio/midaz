#!/bin/bash
# Platform-specific utility functions for Midaz documentation generation
# Provides cross-platform compatibility for timeout operations and other utilities

# Detect platform capabilities
detect_platform_capabilities() {
    local platform=$(uname -s)
    
    # Default capabilities
    HAS_NATIVE_TIMEOUT=false
    HAS_GNU_TIMEOUT=false
    HAS_NATIVE_DOCKER=false
    IS_MACOS=false
    IS_LINUX=false
    IS_WINDOWS=false
    
    case $platform in
        "Darwin")
            IS_MACOS=true
            # Check for Docker Desktop on macOS
            if command -v docker >/dev/null 2>&1; then
                HAS_NATIVE_DOCKER=true
            fi
            ;;
        "Linux")
            IS_LINUX=true
            # Linux typically has native timeout
            if command -v timeout >/dev/null 2>&1; then
                HAS_NATIVE_TIMEOUT=true
            fi
            # Check for Docker
            if command -v docker >/dev/null 2>&1; then
                HAS_NATIVE_DOCKER=true
            fi
            ;;
        CYGWIN*|MINGW*|MSYS*)
            IS_WINDOWS=true
            ;;
    esac
    
    # Check for GNU timeout (might be available via Homebrew on macOS)
    if command -v gtimeout >/dev/null 2>&1; then
        HAS_GNU_TIMEOUT=true
    fi
    
    # On macOS, check if timeout is actually GNU timeout
    if [ "$IS_MACOS" = true ] && command -v timeout >/dev/null 2>&1; then
        # Check if it's the GNU version
        if timeout --version 2>/dev/null | grep -q "GNU"; then
            HAS_GNU_TIMEOUT=true
        fi
    fi
}

# Cross-platform timeout implementation
run_with_timeout() {
    local timeout_duration="$1"
    shift
    local command_to_run="$@"
    
    # Initialize platform detection if not done
    if [ -z "${HAS_NATIVE_TIMEOUT:-}" ]; then
        detect_platform_capabilities
    fi
    
    # Try native timeout first (Linux)
    if [ "$HAS_NATIVE_TIMEOUT" = true ]; then
        timeout "${timeout_duration}" sh -c "$command_to_run"
        return $?
    fi
    
    # Try GNU timeout (if available)
    if [ "$HAS_GNU_TIMEOUT" = true ]; then
        if command -v gtimeout >/dev/null 2>&1; then
            gtimeout "${timeout_duration}" sh -c "$command_to_run"
            return $?
        elif command -v timeout >/dev/null 2>&1; then
            timeout "${timeout_duration}" sh -c "$command_to_run"
            return $?
        fi
    fi
    
    # Fallback: Pure shell implementation for maximum compatibility
    # This works on any POSIX-compliant system
    implement_shell_timeout "${timeout_duration}" "$command_to_run"
}

# Pure shell timeout implementation
implement_shell_timeout() {
    local timeout_duration="$1"
    local command_to_run="$2"
    local temp_dir="/tmp/midaz_timeout_$$"
    local result_file="${temp_dir}/result"
    local pid_file="${temp_dir}/pid"
    
    # Create temp directory
    mkdir -p "${temp_dir}"
    
    # Cleanup function
    cleanup_timeout() {
        if [ -f "${pid_file}" ]; then
            local cmd_pid=$(cat "${pid_file}" 2>/dev/null)
            if [ -n "$cmd_pid" ] && kill -0 "$cmd_pid" 2>/dev/null; then
                kill -TERM "$cmd_pid" 2>/dev/null || true
                sleep 1
                if kill -0 "$cmd_pid" 2>/dev/null; then
                    kill -KILL "$cmd_pid" 2>/dev/null || true
                fi
            fi
        fi
        rm -rf "${temp_dir}" 2>/dev/null || true
    }
    
    # Set trap for cleanup
    trap cleanup_timeout EXIT INT TERM
    
    # Run command in background
    (
        eval "$command_to_run"
        echo $? > "${result_file}"
    ) &
    local cmd_pid=$!
    echo "$cmd_pid" > "${pid_file}"
    
    # Start timeout monitor in background
    (
        sleep "${timeout_duration}"
        if kill -0 "$cmd_pid" 2>/dev/null; then
            echo "124" > "${result_file}" # timeout exit code
            kill -TERM "$cmd_pid" 2>/dev/null || true
            sleep 1
            if kill -0 "$cmd_pid" 2>/dev/null; then
                kill -KILL "$cmd_pid" 2>/dev/null || true
            fi
        fi
    ) &
    local timeout_pid=$!
    
    # Wait for command to complete
    wait "$cmd_pid" 2>/dev/null
    local cmd_exit_code=$?
    
    # Stop timeout monitor
    kill "$timeout_pid" 2>/dev/null || true
    wait "$timeout_pid" 2>/dev/null || true
    
    # Get final result
    local final_result=$cmd_exit_code
    if [ -f "${result_file}" ]; then
        final_result=$(cat "${result_file}" 2>/dev/null || echo "$cmd_exit_code")
    fi
    
    # Cleanup
    cleanup_timeout
    trap - EXIT INT TERM
    
    return "$final_result"
}

# Platform-specific Docker run command with resource constraints
docker_run_with_constraints() {
    local memory_limit="$1"
    local cpu_limit="$2"
    shift 2
    local docker_args="$@"
    
    # Initialize platform detection if not done
    if [ -z "${IS_MACOS:-}" ]; then
        detect_platform_capabilities
    fi
    
    # Base Docker command with resource constraints
    local docker_cmd="docker run --rm"
    
    # Add memory constraint if specified
    if [ -n "$memory_limit" ]; then
        docker_cmd="${docker_cmd} --memory=${memory_limit}"
    fi
    
    # Add CPU constraint if specified
    if [ -n "$cpu_limit" ]; then
        docker_cmd="${docker_cmd} --cpus=${cpu_limit}"
    fi
    
    # Add user mapping for non-root execution
    if [ "$IS_LINUX" = true ]; then
        docker_cmd="${docker_cmd} --user $(id -u):$(id -g)"
    elif [ "$IS_MACOS" = true ]; then
        # Docker Desktop on macOS handles user mapping differently
        docker_cmd="${docker_cmd} --user $(id -u):$(id -g)"
    fi
    
    # Execute Docker command
    eval "${docker_cmd} ${docker_args}"
}

# Get platform-appropriate temp directory
get_temp_dir() {
    if [ "$IS_WINDOWS" = true ]; then
        echo "${TEMP:-/tmp}"
    else
        echo "${TMPDIR:-/tmp}"
    fi
}

# Export functions for use in other scripts
export -f detect_platform_capabilities
export -f run_with_timeout
export -f implement_shell_timeout
export -f docker_run_with_constraints
export -f get_temp_dir

# Initialize platform detection
detect_platform_capabilities