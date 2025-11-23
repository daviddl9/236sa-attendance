.PHONY: fmt test lint build run clean

# Format all Go code
fmt:
	@echo "Formatting Go code..."
	@gofmt -w -s .
	@echo "✅ Formatting complete"

# Run all tests with race detection
test:
	@echo "Running tests with race detection..."
	@go test -race -v ./...
	@echo "✅ Tests complete"

# Run linters
lint:
	@echo "Running Go linters..."
	@go vet ./...
	@gofmt -l . | grep ".*\.go" && exit 1 || exit 0
	@echo "✅ Linting complete"

# Build the application
build:
	@echo "Building application..."
	@go build -o bin/api ./cmd/api
	@echo "✅ Build complete"

# Run the application
run:
	@go run ./cmd/api

# Clean build artifacts
clean:
	@rm -rf bin/
	@echo "✅ Cleaned"

# Install dependencies
deps:
	@go mod download
	@go mod tidy

# Run migrations up
migrate-up:
	@echo "Running migrations..."
	@go run ./cmd/migrate up ./migrations
	@echo "✅ Migrations complete"

# Run migrations down
migrate-down:
	@echo "Rolling back migrations..."
	@go run ./cmd/migrate down ./migrations
	@echo "✅ Migrations rolled back"

# Show migration status
migrate-status:
	@echo "Migration status:"
	@go run ./cmd/migrate status ./migrations

# Development workflow
dev: fmt lint test
	@echo "✅ All checks passed"
