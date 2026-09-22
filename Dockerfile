# ---- Build Stage ----
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 1. Install build tools if needed
RUN apk add --no-cache git

# 2. Download Go module dependencies with layer caching
COPY go.mod go.sum ./
RUN go mod download

# 3. Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY utils/ ./utils/

# 4. Compile static, stripped Linux binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main ./cmd/main/main.go

# 5. Copy migration files
COPY ./migrations ./migrations

# ---- Final Hardened Runtime Stage (DOCKER-01) ----
FROM alpine:3.21

WORKDIR /app

# Install root TLS certificates and timezone data for outbound HTTPS (OpenFDA, AWS S3)
RUN apk --no-cache add ca-certificates tzdata

# Create dedicated non-root unprivileged security group and user
RUN addgroup -g 10001 -S appgroup && \
    adduser -u 10001 -S appuser -G appgroup

# Copy compiled binary and migrations from builder
COPY --from=builder /app/main .
COPY --from=builder /app/migrations ./migrations

# Create local storage directory and ensure correct ownership
RUN mkdir -p /app/storage && \
    chown -R appuser:appgroup /app

# Expose default HTTP server port
EXPOSE 8080

# Switch strictly to unprivileged user
USER appuser:appgroup

# Entrypoint
ENTRYPOINT ["/app/main"]
