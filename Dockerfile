# Build stage
FROM golang:alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod ./
# Run go mod download (if there was a go.sum we would copy it too)
RUN go mod download

# Copy source code
COPY . .

# Build a statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /rewriter main.go

# Final stage - using alpine so we have wget for the healthcheck
FROM alpine:latest

# Create a non-root user for security
RUN adduser -D nonroot

WORKDIR /

# Copy the binary from the builder
COPY --from=builder /rewriter /rewriter

# Expose default port
EXPOSE 8080

# Run as non-root user
USER nonroot:nonroot

# Healthcheck to ensure the container is ready and serving traffic
HEALTHCHECK --interval=30s --timeout=3s \
  CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/rewriter"]
