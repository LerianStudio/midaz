#!/bin/bash

set -e

echo "🧪 Testing cross-platform builds for MT-001..."

# Platforms to test
platforms=("linux/amd64" "darwin/amd64" "darwin/arm64" "windows/amd64")

# Store build results
declare -a build_results
declare -a build_files

for platform in "${platforms[@]}"; do
    os=$(echo $platform | cut -d'/' -f1)
    arch=$(echo $platform | cut -d'/' -f2)
    
    echo "Building for $os/$arch..."
    
    # Set output filename
    output_name="demo-data-$os-$arch"
    if [ "$os" = "windows" ]; then
        output_name="${output_name}.exe"
    fi
    
    # Build for platform
    if GOOS=$os GOARCH=$arch go build -o "$output_name" ./cmd/demo-data; then
        echo "✅ Build successful for $os/$arch"
        build_results+=("SUCCESS")
        build_files+=("$output_name")
        
        # Verify the file was created and has reasonable size
        if [ -f "$output_name" ]; then
            file_size=$(stat -f%z "$output_name" 2>/dev/null || stat -c%s "$output_name" 2>/dev/null || echo "0")
            if [ "$file_size" -gt 1000000 ]; then  # At least 1MB
                echo "   📁 Binary size: $(echo $file_size | awk '{print int($1/1024/1024)"MB"}')"
            else
                echo "   ⚠️  Warning: Binary seems small ($file_size bytes)"
            fi
        else
            echo "   ❌ Binary file not found after build"
            build_results[-1]="FAILED"
        fi
    else
        echo "❌ Build failed for $os/$arch"
        build_results+=("FAILED")
        build_files+=("")
    fi
    
    echo ""
done

echo "📊 Build Summary:"
echo "================="

failed_builds=0
for i in "${!platforms[@]}"; do
    platform="${platforms[$i]}"
    result="${build_results[$i]}"
    file="${build_files[$i]}"
    
    if [ "$result" = "SUCCESS" ]; then
        echo "✅ $platform - $result"
    else
        echo "❌ $platform - $result"
        ((failed_builds++))
    fi
done

echo ""

if [ $failed_builds -eq 0 ]; then
    echo "🎉 All cross-platform builds completed successfully!"
    echo ""
    echo "📦 Generated binaries:"
    for file in "${build_files[@]}"; do
        if [ -n "$file" ] && [ -f "$file" ]; then
            echo "   - $file"
        fi
    done
    
    # Test version command on native binary if available
    echo ""
    echo "🧪 Testing native binary..."
    native_binary=""
    current_os=$(uname -s | tr '[:upper:]' '[:lower:]')
    current_arch=$(uname -m)
    
    # Map architecture names
    case $current_arch in
        x86_64) current_arch="amd64" ;;
        arm64|aarch64) current_arch="arm64" ;;
    esac
    
    for file in "${build_files[@]}"; do
        if [[ "$file" == *"$current_os-$current_arch"* ]]; then
            native_binary="$file"
            break
        fi
    done
    
    if [ -n "$native_binary" ] && [ -f "$native_binary" ]; then
        echo "Testing $native_binary..."
        if ./"$native_binary" version > /dev/null 2>&1; then
            echo "✅ Native binary executes successfully"
        else
            echo "❌ Native binary failed to execute"
            failed_builds=1
        fi
    else
        echo "⚠️  No native binary found for testing"
    fi
    
else
    echo "❌ $failed_builds build(s) failed!"
fi

echo ""
echo "🧹 Cleaning up build artifacts..."
for file in "${build_files[@]}"; do
    if [ -n "$file" ] && [ -f "$file" ]; then
        rm -f "$file"
        echo "   Removed $file"
    fi
done

if [ $failed_builds -eq 0 ]; then
    echo ""
    echo "✅ Cross-platform build test PASSED"
    exit 0
else
    echo ""
    echo "❌ Cross-platform build test FAILED"
    exit 1
fi