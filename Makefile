.PHONY: restart server frontend

# Docker compose restart
restart:
	docker compose down -v
	docker compose up -d --build

# Run the Go backend server
server:
	@cd backend && go run ./cmd/api

# Run the frontend dev server
frontend:
	@cd frontend && npm run dev