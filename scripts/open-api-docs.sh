#!/bin/bash
# Script to open API documentation in browser

echo "🚀 Opening Midaz API Documentation..."
echo "===================================="

# Check if services are running
check_service() {
    local name=$1
    local port=$2
    if nc -z localhost $port 2>/dev/null; then
        echo "✅ $name service is running on port $port"
        return 0
    else
        echo "❌ $name service is not running on port $port"
        return 1
    fi
}

# Check services
onboarding_running=$(check_service "Onboarding" 3000 && echo "true" || echo "false")
transaction_running=$(check_service "Transaction" 3001 && echo "true" || echo "false")

if [[ "$onboarding_running" == "false" ]] && [[ "$transaction_running" == "false" ]]; then
    echo ""
    echo "⚠️  No services are running. Please start them first:"
    echo "   make up-dev"
    exit 1
fi

echo ""
echo "📚 Opening Swagger UI documentation..."

# Detect OS and open browser
open_url() {
    case "$OSTYPE" in
        darwin*)  open "$1" ;;
        linux*)   xdg-open "$1" ;;
        msys*)    start "$1" ;;
        *)        echo "Please open manually: $1" ;;
    esac
}

# Open Swagger UIs
if [[ "$onboarding_running" == "true" ]]; then
    echo "   → Onboarding API: http://localhost:3000/swagger/"
    open_url "http://localhost:3000/swagger/"
fi

if [[ "$transaction_running" == "true" ]]; then
    echo "   → Transaction API: http://localhost:3001/swagger/"
    open_url "http://localhost:3001/swagger/"
fi

echo ""
echo "✨ API documentation opened in your browser!"
echo ""
echo "📝 Additional resources:"
echo "   - OpenAPI specs: /api/openapi.yaml"
echo "   - Swagger JSON: /api/swagger.json"
echo "   - Health checks: /health"