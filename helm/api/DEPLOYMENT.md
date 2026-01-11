# Quick Deployment Guide

## Prerequisites

1. **Docker image is already pushed:** `enivent/tuyul:latest` ✅

2. **Set up cert-manager (for automatic SSL):**
   ```bash
   # Edit cert-manager-issuer.yaml with your email
   kubectl apply -f cert-manager-issuer.yaml
   ```

## Deployment Steps

### 1. Deploy with SSL (cert-manager)

```bash
helm install tuyul-api ./api \
  --namespace playground \
  --create-namespace
```

**Note:** The image is already configured in `values.yaml` as `enivent/tuyul:latest`

### 2. Verify Deployment

```bash
# Check pods
kubectl get pods -n playground

# Check ingress
kubectl get ingress -n playground

# Check certificate (if using cert-manager)
kubectl get certificate -n playground

# Check service
kubectl get svc -n playground
```

### 3. Test the API

```bash
# Health check
curl https://tuyul.envio.co.id/health

# WebSocket endpoint (test with wscat or similar)
wscat -c wss://tuyul.envio.co.id/ws
```

## DNS Configuration

Make sure your domain `tuyul.envio.co.id` points to your Kong Ingress Controller's external IP:

```bash
# Get Kong service external IP
kubectl get svc -n kong-system  # or your Kong namespace

# Add A record in your DNS:
# tuyul.envio.co.id -> <KONG_EXTERNAL_IP>
```

## Troubleshooting

### Certificate not issued

```bash
# Check cert-manager logs
kubectl logs -n cert-manager -l app=cert-manager

# Check certificate status
kubectl describe certificate tuyul-api-tls -n playground

# Check certificate request
kubectl get certificaterequest -n playground
```

### Ingress not working

```bash
# Check ingress
kubectl describe ingress -n playground

# Check Kong logs
kubectl logs -n kong-system -l app=kong
```

### Pod not starting

```bash
# Check pod logs
kubectl logs -n playground -l app.kubernetes.io/name=tuyul-api

# Check pod events
kubectl describe pod -n playground -l app.kubernetes.io/name=tuyul-api
```

## Update Deployment

```bash
# Update image
helm upgrade tuyul-api ./api \
  --namespace playground \
  --set image.tag=new-tag

# Update values
helm upgrade tuyul-api ./api \
  --namespace playground \
  -f updated-values.yaml
```

## Uninstall

```bash
helm uninstall tuyul-api --namespace playground
```

