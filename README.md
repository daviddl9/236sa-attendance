# Go + React Starter Kit

A modern, production-ready full-stack starter kit with Go backend and React (Vite) frontend, featuring local PostgreSQL 17 by default.

## Features

### Backend (Go 1.23.3 + Chi Router)
- **Fast & Type-Safe**: Go's performance with compile-time type checking
- **PostgreSQL 17**: Latest PostgreSQL with optimized connection pooling
- **Authentication**: Email/password + Google OAuth support
- **Session Management**: Secure cookie-based sessions
- **Subscriptions**: Polar payment integration ready
- **API**: RESTful endpoints with proper middleware
- **Docker**: Containerized for easy deployment

### Frontend (React + Vite)
- **Modern React**: React 19 with Vite for fast development
- **TanStack Router**: Type-safe routing with file-based routes
- **UI Components**: shadcn/ui components with Tailwind CSS
- **Type-Safe API**: Connect to Go backend with full type safety
- **Authentication**: Google OAuth integration with session management
- **TanStack Query**: Powerful data fetching and caching

### Infrastructure
- **Docker Compose**: One command to run everything locally
- **Ansible**: Automated production deployment
- **PostgreSQL 17**: Local development with Docker
- **Migrations**: SQL-based schema management

## Quick Start

### Prerequisites
- Go 1.23.3 or later
- Docker & Docker Compose
- Node.js 22.x (for frontend)
- npm or yarn (for frontend dependencies)
- Make (optional, for convenience)

### 1. Clone & Setup

```bash
cd go-nextjs-starter

# Setup backend environment
cd backend
cp .env.example .env
# Edit .env with your configuration

# Setup frontend environment
cd ../frontend
cp .env.example .env
# Edit .env with your backend API URL (default: http://localhost:8080)
```

### 2. Start with Docker Compose

```bash
# Start PostgreSQL + Backend
docker-compose up -d

# Check logs
docker-compose logs -f backend

# Access:
# - Backend API: http://localhost:8080
# - Frontend: http://localhost:5173 (run separately)
# - PostgreSQL: localhost:5432
# - Health check: http://localhost:8080/health
```

### 3. Manual Development (without Docker)

```bash
# Terminal 1: Start PostgreSQL
docker-compose up postgres

# Terminal 2: Run backend
cd backend
cp .env.example .env
go run ./cmd/api

# Terminal 3: Run frontend
cd frontend
npm install
npm run dev
# Frontend will be available at http://localhost:5173
```

## Database Setup

### Auto-initialization with Docker

When using `docker-compose up`, the database schema is automatically created from `backend/migrations/001_initial_schema.sql`.

### Manual Migration

```bash
# Connect to PostgreSQL
psql postgresql://postgres:postgres@localhost:5432/app

# Or use the migration file directly
psql postgresql://postgres:postgres@localhost:5432/app -f backend/migrations/001_initial_schema.sql
```

### Database Schema

The starter includes the following tables:
- `user` - User accounts
- `session` - Authentication sessions
- `account` - OAuth provider accounts
- `verification` - Email verification tokens
- `subscription` - Polar payment subscriptions

## API Endpoints

### Authentication
- `POST /api/auth/sign-up` - Create new account
- `POST /api/auth/sign-in` - Sign in with email/password
- `POST /api/auth/sign-out` - Sign out
- `GET /api/auth/session` - Get current session
- `GET /api/auth/oauth/google` - Google OAuth redirect
- `GET /api/auth/oauth/callback` - OAuth callback

### Protected Routes (require authentication)
- `POST /api/chat` - OpenAI chat streaming (TODO)
- `GET /api/user/subscription` - Get user subscription (TODO)

### Webhooks
- `POST /api/webhooks/polar` - Polar subscription webhooks (TODO)

## Development

### Backend

```bash
cd backend

# Run tests
go test -race ./...

# Run with hot reload (install air first)
go install github.com/air-verse/air@latest
air

# Build for production
go build -o bin/api ./cmd/api

# Format code
go fmt ./...

# Lint
go vet ./...
```

### Frontend

```bash
cd frontend

# Install dependencies
npm install

# Development server (runs on http://localhost:5173)
npm run dev

# Build for production
npm run build

# Preview production build
npm run preview

# Generate route types (after adding new routes)
npm run generate
```

## Production Deployment

### Using Ansible

```bash
cd ansible

# Update inventory with your server details
vim inventory.yml

# Deploy to production
ansible-playbook -i inventory.yml playbook.yml
```

### Manual Deployment

1. **Build backend**:
```bash
cd backend
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app ./cmd/api
```

2. **Setup PostgreSQL** on your server (version 17)

3. **Run migrations**:
```bash
psql $DATABASE_URL -f migrations/001_initial_schema.sql
```

4. **Start the application**:
```bash
./app
```

## Environment Variables

### Backend

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Yes | - |
| `PORT` | Server port | No | 8080 |
| `FRONTEND_URL` | Frontend URL for CORS and OAuth redirects | Yes | http://localhost:5173 |
| `BACKEND_URL` | Backend URL for OAuth callbacks | Yes | http://localhost:8080 |
| `ENVIRONMENT` | development/production | No | development |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID | No | - |
| `GOOGLE_CLIENT_SECRET` | Google OAuth secret | No | - |
| `OPENAI_API_KEY` | OpenAI API key | No | - |
| `POLAR_ACCESS_TOKEN` | Polar API token | No | - |
| `POLAR_WEBHOOK_SECRET` | Polar webhook secret | No | - |

### Frontend

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `VITE_API_URL` | Backend API URL | Yes | http://localhost:8080 |

## Project Structure

```
.
├── backend/
│   ├── cmd/
│   │   └── api/           # Application entrypoint
│   ├── internal/
│   │   ├── database/      # PostgreSQL connection
│   │   ├── handlers/      # HTTP handlers
│   │   ├── middleware/    # Auth, CORS, etc.
│   │   ├── models/        # Data models
│   │   └── services/      # Business logic
│   ├── migrations/        # SQL migrations
│   ├── Dockerfile
│   ├── Makefile
│   └── go.mod
├── frontend/              # React + Vite application
│   ├── src/
│   │   ├── routes/       # TanStack Router routes
│   │   ├── components/   # React components
│   │   ├── lib/          # Utilities and API client
│   │   └── hooks/        # Custom React hooks
│   ├── vite.config.ts
│   └── package.json
├── ansible/               # Deployment automation
│   ├── playbook.yml
│   ├── inventory.yml
│   ├── roles/
│   ├── group_vars/
│   └── templates/
├── docker-compose.yml
└── README.md
```

## Security Features

- **Password Hashing**: bcrypt with appropriate cost
- **Secure Sessions**: HttpOnly cookies with SameSite protection
- **CSRF Protection**: Proper CORS configuration
- **SQL Injection**: Parameterized queries with pgx
- **Session Expiry**: 30-day sessions with automatic cleanup

## Performance

- **Connection Pooling**: pgx pool (5-25 connections)
- **Compile-time Safety**: Go's type system
- **Low Latency**: Chi router with minimal middleware
- **Efficient Database**: PostgreSQL 17 with proper indexes

## Next Steps

### Completed ✅
- [x] Go backend with Chi router
- [x] PostgreSQL 17 schema
- [x] Authentication (sign-up, sign-in, sign-out, Google OAuth)
- [x] Session middleware
- [x] Docker Compose setup
- [x] React + Vite frontend
- [x] TanStack Router setup
- [x] shadcn/ui components
- [x] Dashboard with charts
- [x] Environment configuration

### TODO
- [ ] OpenAI chat streaming endpoint
- [ ] Polar webhook integration
- [ ] Production deployment guide for React SPA
- [ ] Ansible playbook completion

## Contributing

This is a starter template. Feel free to:
- Add new features
- Improve error handling
- Add tests
- Enhance documentation

## License

MIT

## Support

For issues and questions, please open an issue in the repository.
