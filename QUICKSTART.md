# ⚡ Quickstart Guide

Get the full stack running in under 5 minutes.

## Prerequisites

- Docker & Docker Compose
- Go 1.23.3+ (for local development without Docker)
- Node.js 22+ (for frontend)

## 🚀 Fast Start (Docker)

```bash
# 1. Clone or navigate to the repo
cd go-nextjs-starter

# 2. Set up environment files
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env

# 3. Start everything with Docker Compose
docker-compose up

# Backend will be at: http://localhost:8080
# PostgreSQL will be at: localhost:5432
```

## 🎨 Start Frontend (separate terminal)

```bash
# Navigate to frontend
cd frontend

# Install dependencies (first time only)
npm install

# Start development server
npm run dev

# Frontend will be at: http://localhost:3000
```

## ✅ Verify It's Working

1. Visit **http://localhost:3000**
2. Click "Sign Up" and create an account
3. You should be redirected to the dashboard
4. Try the chat feature (requires `OPENAI_API_KEY` in backend/.env)

## 🔧 Local Development (without Docker)

### Terminal 1: Database
```bash
docker-compose up postgres
```

### Terminal 2: Backend
```bash
cd backend
cp .env.example .env
go run ./cmd/api
```

### Terminal 3: Frontend
```bash
cd frontend
npm install
npm run dev
```

## 📝 Environment Variables

### Backend (`backend/.env`)

**Required:**
```env
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/app?sslmode=disable
PORT=8080
FRONTEND_URL=http://localhost:3000
```

**Optional Features:**
```env
# OpenAI Chat (optional)
OPENAI_API_KEY=sk-...

# Google OAuth (optional)
GOOGLE_CLIENT_ID=your-client-id
GOOGLE_CLIENT_SECRET=your-secret

# Polar Subscriptions (optional)
POLAR_ACCESS_TOKEN=polar_...
POLAR_WEBHOOK_SECRET=whsec_...
```

### Frontend (`frontend/.env`)

```env
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_APP_URL=http://localhost:3000
```

## 🎯 Common Tasks

### Run Database Migrations

Migrations run automatically with Docker Compose. To run manually:

```bash
psql postgresql://postgres:postgres@localhost:5432/app -f backend/migrations/001_initial_schema.sql
```

### Reset Database

```bash
docker-compose down -v
docker-compose up
```

### Build for Production

**Backend:**
```bash
cd backend
go build -o bin/api ./cmd/api
./bin/api
```

**Frontend:**
```bash
cd frontend
npm run build
npm start
```

### Run Tests

**Backend:**
```bash
cd backend
go test -race ./...
```

**Frontend:**
```bash
cd frontend
npm run lint
```

## 🔑 Default Credentials

**PostgreSQL:**
- Host: localhost
- Port: 5432
- Database: app
- User: postgres
- Password: postgres

## 🆘 Troubleshooting

### Port already in use
```bash
# Check what's using port 8080 or 3000
lsof -ti:8080
lsof -ti:3000

# Kill the process
kill -9 $(lsof -ti:8080)
```

### Database connection refused
```bash
# Make sure PostgreSQL is running
docker-compose ps

# Restart just the database
docker-compose restart postgres
```

### Frontend won't start
```bash
# Clear Next.js cache
cd frontend
rm -rf .next node_modules
npm install
```

### Backend compilation errors
```bash
cd backend
go mod tidy
go mod download
```

## 🎉 What's Included

✅ **Authentication**
- Email/password sign up/in
- Google OAuth
- Session management
- Protected routes

✅ **Features**
- OpenAI chat streaming
- Polar payment webhooks
- PostgreSQL 17 database
- Beautiful dashboard UI

✅ **DevOps**
- Docker Compose setup
- Ansible deployment
- Production-ready configs

## 📚 Next Steps

- Read the full [README.md](README.md) for detailed documentation
- Check [ansible/](ansible/) for deployment automation
- Explore the API at http://localhost:8080/health

## 🐛 Issues?

If something doesn't work:

1. Check all services are running: `docker-compose ps`
2. View logs: `docker-compose logs -f backend`
3. Verify environment variables are set correctly
4. Try: `docker-compose down && docker-compose up --build`

---

**Need help?** Open an issue in the repository.
