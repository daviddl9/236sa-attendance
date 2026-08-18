# 236SA Attendance

## Infrastructure

### Server Access
- **Host**: `redcon.236sa.one`
- **SSH Key**: `~/Projects/236sa-cloud/infra/deploy-key.pem`
- **User**: `ubuntu`
- **SSH command**: `ssh -i ~/Projects/236sa-cloud/infra/deploy-key.pem ubuntu@redcon.236sa.one`

### Debugging Production
```bash
# SSH into server
ssh -i ~/Projects/236sa-cloud/infra/deploy-key.pem ubuntu@redcon.236sa.one

# View API container logs
sudo docker logs apps-attendance-api-1 --tail 100

# Follow logs in real-time
sudo docker logs apps-attendance-api-1 --follow

# Restart the API container
sudo docker compose -f /opt/apps/docker-compose.yml restart attendance-api

# Check container status
sudo docker ps | grep attendance
```

### Deploy
- Push to `main` triggers GitHub Actions deploy (`.github/workflows/deploy.yml`)
- Builds Go backend + Vite frontend, rsync to server, restarts container
- Monitor deploy: `gh run list --limit 1` / `gh run watch <id>`

## Project Structure
- `backend/` — Go API server (port 8081 in prod)
- `frontend/` — Vite + React SPA
- `docker-compose.yml` — local dev (PostgreSQL + backend)

## Git
- Sign off as David (ddl.tdh@gmail.com)
- No co-authored-by lines in commit messages

<!-- SPECKIT START -->
Current spec-kit plan: specs/010-ops-groups/plan.md
<!-- SPECKIT END -->
