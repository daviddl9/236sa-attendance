# 236SA Attendance System - Infrastructure

Single-command deployment for reservist call-ups using AWS EC2 with hourly billing.

## Quick Reference

```bash
make infra-deploy   # Spin up everything (~2 min)
make infra-destroy  # Tear down (stops billing)
make infra-status   # Check current state
make infra-ssh      # SSH into server
make infra-logs     # Tail startup logs
```

## Architecture

- **Platform**: AWS EC2 (ap-southeast-1 / Singapore)
- **Instance**: t3.small (2 vCPU, 2GB RAM) - ~$0.021/hour
- **OS**: Ubuntu 24.04 LTS
- **Storage**: 20GB gp3 EBS
- **Services**: PostgreSQL 17, Go backend, React frontend, Nginx

## Cost Breakdown

| Resource | Cost |
|----------|------|
| EC2 t3.small (running) | ~$0.50/day |
| EC2 (when destroyed) | $0 |
| Route 53 hosted zone | ~$0.50/month |
| Domain (annual) | ~$12/year |

**Example: 3 call-ups × 2 weeks = ~$30/year total**

---

## One-Time Setup

### 1. Install AWS CLI

```bash
brew install awscli
```

### 2. Configure AWS Credentials

```bash
aws configure
```

Enter:
- AWS Access Key ID
- AWS Secret Access Key
- Default region: `ap-southeast-1`
- Default output: `json`

**Need credentials?** Create an IAM user with `AmazonEC2FullAccess` and `AmazonRoute53FullAccess` policies.

### 3. Register Domain

**Option A: AWS Route 53** (recommended)
1. Go to [Route 53 Console](https://console.aws.amazon.com/route53/) → Registered Domains
2. Register `236sa.one` (~$12/year)
3. Disable auto-renew if desired
4. Note the **Hosted Zone ID** (created automatically)

**Option B: External Registrar**
1. Register domain elsewhere (Namecheap, Cloudflare, etc.)
2. Create hosted zone:
   ```bash
   aws route53 create-hosted-zone --name 236sa.one --caller-reference $(date +%s)
   ```
3. Update domain nameservers to Route 53's NS records

### 4. Push Code to GitHub

```bash
gh repo create 236sa-attendance --private --source=. --push
```

### 5. Configure Scripts

Edit `infrastructure/scripts/config.sh`:

```bash
# Required - get from Route 53 console
export HOSTED_ZONE_ID="Z1234567890ABC"

# Required - your GitHub repo
export GIT_REPO="https://github.com/yourusername/attendance.git"

# Required - database password
export DB_PASSWORD="your-secure-password-here"
```

---

## Deployment Workflow

### Before Each Call-Up

```bash
# Deploy infrastructure
make infra-deploy

# Wait for startup (10-15 min), monitor progress
make infra-logs

# After startup completes, set up SSL
make infra-ssh
sudo certbot --nginx -d 236sa.one
```

### After Each Call-Up

```bash
# Tear down to stop billing
make infra-destroy
```

---

## Make Targets

| Command | Description |
|---------|-------------|
| `make infra-deploy` | Create EC2 instance, security group, elastic IP, DNS |
| `make infra-destroy` | Terminate instance, release elastic IP |
| `make infra-status` | Show current infrastructure state |
| `make infra-ssh` | SSH into the running server |
| `make infra-logs` | Tail the startup script logs |

---

## Files

```
infrastructure/
├── README.md           # This file
├── scripts/
│   ├── config.sh       # Configuration (edit this!)
│   ├── deploy.sh       # Deployment script
│   ├── destroy.sh      # Teardown script
│   ├── status.sh       # Status check
│   └── startup.sh      # Server initialization (runs on boot)
├── terraform/          # Legacy Terraform setup (optional)
└── ansible/            # Legacy Ansible playbooks (optional)
```

---

## What Gets Deployed

The startup script automatically:
1. Installs PostgreSQL 17, Go 1.23, Node.js 22, Nginx
2. Clones your application from GitHub
3. Builds the Go backend and React frontend
4. Runs database migrations
5. Configures Nginx reverse proxy
6. Sets up systemd service for auto-restart
7. Prepares for SSL (manual certbot step)

---

## What Gets Preserved Between Deployments

| Resource | Preserved? | Notes |
|----------|------------|-------|
| Domain registration | Yes | Manual renewal required |
| Route 53 hosted zone | Yes | ~$0.50/month |
| Security group | Yes | Reused on next deploy |
| SSH key pair | Yes | Reused on next deploy |
| EC2 instance | No | Terminated to stop billing |
| Elastic IP | No | Released to avoid charges |
| EBS volume | No | Deleted with instance |

---

## Troubleshooting

### Startup Script Not Completing

```bash
make infra-ssh
tail -f /var/log/startup-script.log
```

### Application Not Starting

```bash
make infra-ssh
sudo systemctl status attendance
sudo journalctl -u attendance -f
```

### Database Issues

```bash
make infra-ssh
sudo -u postgres psql -d attendance
```

### SSL Certificate Issues

```bash
make infra-ssh
sudo certbot --nginx -d 236sa.one --force-renewal
```

### Out of Memory

The system has 4GB swap. Check usage:
```bash
make infra-ssh
free -h
htop
```

---

## Security Notes

- SSH key (`attendance-key.pem`) auto-generated and gitignored
- Security group allows only ports 22, 80, 443
- PostgreSQL only accessible from localhost
- Database password stored in config.sh (gitignored)

---

## Legacy: Terraform Setup

The `terraform/` directory contains an alternative Terraform-based setup. Use this if you prefer infrastructure-as-code with state management.

See [terraform/README.md](terraform/README.md) for details.
