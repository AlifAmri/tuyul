# TUYUL API Helm Chart

This Helm chart deploys the TUYUL Trading Bot API to a Kubernetes cluster.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- Redis database (can be external or in-cluster)
- Docker (for building the container image)
- Container registry (Docker Hub, ECR, GCR, etc.) - optional if using local images
- Kong Ingress Controller installed in your cluster
- cert-manager (optional, for automatic SSL certificates via Let's Encrypt)

## Building the Docker Image

Before deploying with Helm, you need to build and push the Docker image:

### Using Makefile (Recommended)

```bash
# Build the image
cd api
make docker-build

# Build and push to Docker Hub
make docker-push DOCKER_USERNAME=your-username

# Or build with custom tag
make docker-build IMAGE_TAG=v1.0.0
make docker-push DOCKER_USERNAME=your-username IMAGE_TAG=v1.0.0
```

### Using Docker directly

```bash
# Build the image
cd backend
docker build -t tuyul-backend:latest .

# Tag for Docker Hub
docker tag tuyul-backend:latest your-username/tuyul-backend:latest

# Login to Docker Hub (if not already logged in)
docker login

# Push to Docker Hub
docker push your-username/tuyul-backend:latest
```

### Update Helm values with your Docker Hub image

```yaml
# values.yaml or --set flag
image:
  repository: your-username/indodax-watcher-api
  tag: latest
  pullPolicy: Always
```

## SSL/TLS Configuration

The chart is pre-configured for SSL with domain `idax.envio.co.id` in the `playground` namespace.

### Option 1: Automatic SSL with cert-manager (Recommended)

If you have cert-manager installed, the chart will automatically request a Let's Encrypt certificate:

```bash
# Ensure cert-manager ClusterIssuer exists
# Example ClusterIssuer (create this separately):
# kubectl apply -f - <<EOF
# apiVersion: cert-manager.io/v1
# kind: ClusterIssuer
# metadata:
#   name: letsencrypt-prod
# spec:
#   acme:
#     server: https://acme-v02.api.letsencrypt.org/directory
#     email: your-email@example.com
#     privateKeySecretRef:
#       name: letsencrypt-prod
#     solvers:
#     - http01:
#         ingress:
#           class: kong
# EOF

# Deploy with cert-manager
helm install indodax-watcher-api ./helm/api \
  --namespace playground \
  --create-namespace \
  --set image.repository=your-username/indodax-watcher-api \
  --set image.tag=latest \
  --set database.url="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

### Option 2: Manual SSL Certificate

If you have an existing TLS certificate:

```bash
# Create TLS secret manually
kubectl create secret tls indodax-watcher-api-tls \
  --cert=path/to/cert.crt \
  --key=path/to/cert.key \
  -n playground

# Deploy (certificate will be used automatically)
helm install indodax-watcher-api ./helm/api \
  --namespace playground \
  --create-namespace \
  --set image.repository=your-username/indodax-watcher-api \
  --set image.tag=latest \
  --set database.url="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

### Option 3: Custom Domain

To use a different domain, update values.yaml or use --set:

```bash
helm install indodax-watcher-api ./helm/api \
  --namespace playground \
  --create-namespace \
  --set ingress.hosts[0].host=your-domain.com \
  --set ingress.tls[0].hosts[0]=your-domain.com \
  --set image.repository=your-username/indodax-watcher-api \
  --set database.url="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

## Installation

### Basic Installation (with SSL)

```bash
# Deploy with Docker Hub image, SSL enabled, in playground namespace
helm install indodax-watcher-api ./helm/api \
  --namespace playground \
  --create-namespace \
  --set image.repository=your-username/indodax-watcher-api \
  --set image.tag=latest \
  --set database.url="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

### With Custom Values

```bash
helm install indodax-watcher-api ./helm/api -f my-values.yaml
```

### Using DATABASE_URL

```yaml
# my-values.yaml
database:
  url: "postgres://user:password@host:5432/dbname?sslmode=disable"
```

### Using Individual Database Parameters

```yaml
# my-values.yaml
database:
  host: "postgres-service"
  port: "5432"
  user: "postgres"
  password: "your-password"
  dbName: "ix_screener"
  sslMode: "disable"
```

### Using Secrets for Sensitive Data

```yaml
# my-values.yaml
secrets:
  databasePassword: "your-database-password"
  indodaxToken: "your-indodax-token"
```

## Configuration

The following table lists the configurable parameters:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Container image repository | `indodax-watcher-api` |
| `image.tag` | Container image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Service port | `8080` |
| `database.url` | PostgreSQL connection string | `""` |
| `database.host` | Database host | `""` |
| `database.port` | Database port | `5432` |
| `database.user` | Database user | `postgres` |
| `database.password` | Database password | `""` |
| `database.dbName` | Database name | `ix_screener` |
| `database.sslMode` | SSL mode | `disable` |
| `indodax.restUrl` | Indodax REST API URL | `https://indodax.com` |
| `indodax.wsUrl` | Indodax WebSocket URL | `wss://ws3.indodax.com/ws/` |
| `indodax.staticToken` | Indodax static token | (default token) |
| `server.port` | Server port | `8080` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `512Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |
| `namespace` | Kubernetes namespace | `playground` |
| `ingress.enabled` | Enable ingress | `true` |
| `ingress.className` | Ingress class name | `kong` |
| `ingress.annotations` | Ingress annotations (Kong-specific) | See values.yaml |
| `ingress.hosts` | Ingress hosts | `idax.envio.co.id` |
| `ingress.tls` | TLS configuration | Pre-configured for SSL |
| `autoscaling.enabled` | Enable HPA | `false` |

## Examples

### Deploy with External PostgreSQL

```bash
helm install indodax-watcher-api ./helm/api \
  --set database.host=postgres.example.com \
  --set database.user=postgres \
  --set database.dbName=ix_screener \
  --set secrets.databasePassword=my-secure-password
```

### Deploy with Kong Ingress and SSL

```yaml
# values.yaml
namespace: playground

ingress:
  enabled: true
  className: "kong"
  annotations:
    # Include ws,wss for WebSocket support (API has /ws endpoint)
    konghq.com/protocols: "http,https,ws,wss"
    konghq.com/strip-path: "false"
    konghq.com/preserve-host: "true"
    # SSL/TLS with cert-manager
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    # Optional: Add Kong plugins (comma-separated)
    # konghq.com/plugins: "rate-limiting,cors"
  hosts:
    - host: idax.envio.co.id
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: indodax-watcher-api-tls
      hosts:
        - idax.envio.co.id
```

### Deploy with Kong Ingress and Custom Plugins

```yaml
# values.yaml
ingress:
  enabled: true
  className: "kong"
  annotations:
    konghq.com/protocols: "http,https"
    konghq.com/strip-path: "false"
    konghq.com/preserve-host: "true"
    konghq.com/plugins: "rate-limiting,cors,request-id"
  hosts:
    - host: api.example.com
      paths:
        - path: /
          pathType: Prefix
```

### Enable Autoscaling

```yaml
# values.yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 80
```

## Health Checks

The deployment includes:
- **Liveness Probe**: `/health` endpoint, checks every 10 seconds
- **Readiness Probe**: `/health` endpoint, checks every 5 seconds

## Uninstallation

```bash
helm uninstall indodax-watcher-api
```

