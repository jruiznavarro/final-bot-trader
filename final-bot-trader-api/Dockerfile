# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the applications
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/live-trading
RUN CGO_ENABLED=0 GOOS=linux go build -o /monitor ./cmd/monitor

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Copy binaries from builder
COPY --from=builder /bot /app/bot
COPY --from=builder /monitor /app/monitor

# Create directory for state file
RUN mkdir -p /app/data

# Environment variables (override in docker-compose)
ENV BITUNIX_API_KEY=""
ENV BITUNIX_SECRET_KEY=""
ENV TELEGRAM_BOT_TOKEN=""
ENV TELEGRAM_CHAT_ID=""
ENV DB_HOST="postgres"
ENV DB_PORT="5432"
ENV DB_USER="trader"
ENV DB_PASSWORD="trader_password"
ENV DB_NAME="trading_bot"
ENV DB_SSLMODE="disable"
ENV API_PORT="8080"
ENV TZ="Europe/Madrid"

# Expose API port
EXPOSE 8080

# Default command
CMD ["/app/bot", "--auto-confirm"]
