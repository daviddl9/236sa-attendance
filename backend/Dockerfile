# Build stage
FROM golang:1.25.3-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY backend/ ./backend/

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./backend/cmd/api

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates postgresql-client

WORKDIR /root/

# Copy binary and migrations from builder
COPY --from=builder /app/main .
COPY --from=builder /app/backend/migrations ./migrations

# Expose port
EXPOSE 8080

# Run the application (migrations run automatically on startup)
CMD ["./main"]
