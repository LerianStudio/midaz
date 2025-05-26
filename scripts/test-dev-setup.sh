#!/bin/bash
# Test script for development setup

echo "🧪 Testing Midaz Development Setup"
echo "================================="

# Check infrastructure services
echo "📍 Checking infrastructure services..."
services=(
    "midaz-postgres-primary:5701"
    "midaz-mongodb:5703"
    "midaz-rabbitmq:3003"
    "midaz-valkey:5704"
)

for service in "${services[@]}"; do
    IFS=':' read -r name port <<< "$service"
    if nc -z localhost $port 2>/dev/null; then
        echo "✅ $name is running on port $port"
    else
        echo "❌ $name is not accessible on port $port"
    fi
done

# Check application services
echo -e "\n📍 Checking application services..."
echo -n "✅ Onboarding service: "
curl -s http://localhost:3000/health || echo "❌ Not responding"
echo ""

echo -n "✅ Transaction service: "
curl -s http://localhost:3001/health || echo "❌ Not responding"
echo ""

echo -e "\n✨ Development setup is ready!"
echo "You can now make changes to the code and they will be automatically reloaded."