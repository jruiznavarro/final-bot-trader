#!/bin/bash
# Deploy script for Copy Trading Bot

set -e

echo "========================================="
echo "  Copy Trading Bot - Deployment"
echo "========================================="
echo

# Check if .env exists
if [ ! -f .env ]; then
    echo "Creating .env file from template..."
    cp .env.example .env
    echo ""
    echo "IMPORTANT: Edit .env file with your credentials:"
    echo "  nano .env"
    echo ""
    echo "Then run this script again."
    exit 1
fi

# Check if credentials are set
if grep -q "your_api_key_here" .env; then
    echo "ERROR: Please configure your API credentials in .env file"
    echo "  nano .env"
    exit 1
fi

echo "Building containers..."
docker-compose build

echo ""
echo "Starting services..."
docker-compose up -d

echo ""
echo "Waiting for services to start..."
sleep 5

echo ""
echo "========================================="
echo "  Deployment Complete!"
echo "========================================="
echo ""
echo "Services:"
docker-compose ps
echo ""
echo "Access:"
echo "  Dashboard: http://$(hostname -I | awk '{print $1}'):3001"
echo "  API:       http://$(hostname -I | awk '{print $1}'):8080/health"
echo ""
echo "Logs:"
echo "  docker-compose logs -f bot"
echo ""
