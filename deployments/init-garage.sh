#!/bin/bash
# Garage S3 Initialization Script
# Run this after starting docker-compose to initialize Garage

set -e

echo "🔧 Initializing Garage S3..."

# Wait for Garage to be ready
echo "⏳ Waiting for Garage to start..."
sleep 3

# Get node ID
echo "📋 Getting node ID..."
NODE_ID=$(docker exec epbf-garage /garage node id | grep -oP '^[a-f0-9]+')
echo "   Node ID: $NODE_ID"

# Short ID (first 16 chars)
SHORT_ID=${NODE_ID:0:16}

# Assign layout
echo "📐 Assigning cluster layout..."
docker exec epbf-garage /garage layout assign "$SHORT_ID" -z default -c 10G > /dev/null

# Apply layout
echo "✅ Applying layout..."
docker exec epbf-garage /garage layout apply --version 1 > /dev/null

# Create bucket
echo "🪣 Creating bucket 'epbf-plugins'..."
docker exec epbf-garage /garage bucket create epbf-plugins > /dev/null

# Create admin key
echo "🔑 Creating admin access key..."
KEY_OUTPUT=$(docker exec epbf-garage /garage key create epbf-admin)
KEY_ID=$(echo "$KEY_OUTPUT" | grep "Key ID:" | awk '{print $NF}')
SECRET_KEY=$(echo "$KEY_OUTPUT" | grep "Secret key:" | awk '{print $NF}')

# Grant permissions
echo "🔐 Granting permissions..."
docker exec epbf-garage /garage key allow "$KEY_ID" --create-bucket > /dev/null

echo ""
echo "✅ Garage S3 initialized successfully!"
echo ""
echo "📝 Connection details:"
echo "   Endpoint:    http://127.0.0.1:3900"
echo "   Region:      garage"
echo "   Bucket:      epbf-plugins"
echo "   Key ID:      $KEY_ID"
echo "   Secret Key:  $SECRET_KEY"
echo ""
echo "💾 Save these credentials to your environment:"
echo "   export S3_ENDPOINT=http://127.0.0.1:3900"
echo "   export S3_REGION=garage"
echo "   export S3_ACCESS_KEY=$KEY_ID"
echo "   export S3_SECRET_KEY=$SECRET_KEY"
echo "   export S3_BUCKET=epbf-plugins"
echo ""
