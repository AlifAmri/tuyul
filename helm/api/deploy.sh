#!/bin/bash
# Quick deployment script for TUYUL API

set -e

echo "🚀 Deploying TUYUL API to Kubernetes..."

# Deploy with Helm
helm upgrade --install tuyul-api ./api \
  --namespace playground \
  --create-namespace \
  --wait \
  --timeout 5m

echo "✅ Deployment complete!"
echo ""
echo "📋 Check deployment status:"
echo "   kubectl get pods -n playground"
echo "   kubectl get ingress -n playground"
echo ""
echo "🌐 API will be available at: https://tuyul.envio.co.id"
