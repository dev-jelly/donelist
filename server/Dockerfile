# Multi-stage build for optimized production image

# Stage 1: Builder
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum for dependency caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s -extldflags '-static'" \
    -o donelist-api ./cmd/api

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1001 -S donelist && \
    adduser -u 1001 -S donelist -G donelist

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/donelist-api /app/donelist-api

# Copy migrations (needed for runtime)
COPY --from=builder /app/migrations /app/migrations

# Change ownership to non-root user
RUN chown -R donelist:donelist /app

# Switch to non-root user
USER donelist

# Expose application port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["/app/donelist-api"]