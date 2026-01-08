# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/tuyul-backend ./cmd/api/main.go

# Final stage
FROM alpine:latest

# Add ca-certificates for HTTPS requests to Indodax
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/tuyul-backend .

# Copy .env.example if needed (or rely on env vars in docker-compose)
# COPY .env.example .env

# Expose the application port
EXPOSE 8080

# Command to run the application
CMD ["./tuyul-backend"]
