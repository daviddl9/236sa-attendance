#!/bin/bash
set -e

# Log all output
exec > >(tee /var/log/startup-script.log)
exec 2>&1

echo "=== Starting 236SA Attendance System Setup at $(date) ==="

# Configuration - these are passed via environment or hardcoded for now
# In production, these should be managed via secrets manager
GIT_REPO="${GIT_REPO:-}"
GIT_BRANCH="${GIT_BRANCH:-main}"
DOMAIN="${DOMAIN:-236sa.one}"
DB_NAME="${DB_NAME:-attendance}"
DB_USER="${DB_USER:-attendance}"
DB_PASSWORD="${DB_PASSWORD:-}"

# Update system packages
echo ""
echo "=== Updating system packages ==="
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get upgrade -y

# Install base dependencies
echo ""
echo "=== Installing base dependencies ==="
apt-get install -y \
    curl \
    wget \
    git \
    build-essential \
    software-properties-common \
    apt-transport-https \
    ca-certificates \
    gnupg \
    lsb-release \
    python3 \
    python3-pip \
    unzip \
    jq

# Install PostgreSQL 17
echo ""
echo "=== Installing PostgreSQL 17 ==="
sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add -
apt-get update
apt-get install -y postgresql-17 postgresql-contrib-17

# Configure PostgreSQL for 2GB RAM
echo ""
echo "=== Configuring PostgreSQL ==="
cat >> /etc/postgresql/17/main/postgresql.conf <<EOF

# Custom configuration for 2GB RAM system
shared_buffers = 512MB
effective_cache_size = 1GB
work_mem = 4MB
maintenance_work_mem = 64MB
max_connections = 50
wal_buffers = 16MB
checkpoint_completion_target = 0.9
random_page_cost = 1.1
EOF

systemctl restart postgresql
systemctl enable postgresql

# Create database and user
echo ""
echo "=== Setting up database ==="
sudo -u postgres psql <<EOF
CREATE USER $DB_USER WITH PASSWORD '$DB_PASSWORD';
CREATE DATABASE $DB_NAME OWNER $DB_USER;
GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;
EOF

echo "PostgreSQL 17 installed and configured"

# Install Go 1.23
echo ""
echo "=== Installing Go 1.23 ==="
wget -q https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
rm go1.23.4.linux-amd64.tar.gz

echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/bash.bashrc
export PATH=$PATH:/usr/local/go/bin

echo "Go installed: $(go version)"

# Install Node.js 22
echo ""
echo "=== Installing Node.js 22 ==="
curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
apt-get install -y nodejs

echo "Node.js installed: $(node --version)"

# Install Nginx
echo ""
echo "=== Installing Nginx ==="
apt-get install -y nginx
systemctl enable nginx

# Install certbot for Let's Encrypt
echo ""
echo "=== Installing Certbot ==="
apt-get install -y certbot python3-certbot-nginx

# Create application user
echo ""
echo "=== Creating application user ==="
useradd -r -m -s /bin/bash -d /opt/attendance attendance || true

# Create application directories
mkdir -p /opt/attendance/{backend,frontend}
mkdir -p /var/www/attendance
chown -R attendance:attendance /opt/attendance
chown -R attendance:attendance /var/www/attendance

# Create 4GB swap file
echo ""
echo "=== Creating swap file ==="
if [ ! -f /swapfile ]; then
    fallocate -l 4G /swapfile
    chmod 600 /swapfile
    mkswap /swapfile
    swapon /swapfile
    echo '/swapfile none swap sw 0 0' >> /etc/fstab
    echo "Swap file created and enabled"
else
    echo "Swap file already exists"
fi

# Clone and build application (if GIT_REPO is set)
if [ -n "$GIT_REPO" ]; then
    echo ""
    echo "=== Cloning application ==="
    sudo -u attendance git clone --branch "$GIT_BRANCH" "$GIT_REPO" /opt/attendance/app

    # Build backend
    echo ""
    echo "=== Building backend ==="
    cd /opt/attendance/app/backend
    sudo -u attendance bash -c 'export PATH=$PATH:/usr/local/go/bin && go mod download && go build -o bin/api ./cmd/api'

    # Create backend environment file
    cat > /opt/attendance/app/backend/.env <<EOF
DATABASE_URL=postgres://$DB_USER:$DB_PASSWORD@localhost:5432/$DB_NAME?sslmode=disable
PORT=8080
FRONTEND_URL=https://$DOMAIN
EOF
    chown attendance:attendance /opt/attendance/app/backend/.env
    chmod 600 /opt/attendance/app/backend/.env

    # Run migrations
    echo ""
    echo "=== Running database migrations ==="
    cd /opt/attendance/app/backend
    PGPASSWORD="$DB_PASSWORD" psql -h localhost -U "$DB_USER" -d "$DB_NAME" -f migrations/001_initial_schema.sql || true

    # Build frontend
    echo ""
    echo "=== Building frontend ==="
    cd /opt/attendance/app/frontend
    sudo -u attendance npm ci
    sudo -u attendance npm run build

    # Copy frontend build to web root
    cp -r /opt/attendance/app/frontend/dist/* /var/www/attendance/
    chown -R www-data:www-data /var/www/attendance

    # Create systemd service
    echo ""
    echo "=== Creating systemd service ==="
    cat > /etc/systemd/system/attendance.service <<EOF
[Unit]
Description=236SA Attendance System
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=attendance
Group=attendance
WorkingDirectory=/opt/attendance/app/backend
ExecStart=/opt/attendance/app/backend/bin/api
Restart=always
RestartSec=5
Environment=PATH=/usr/local/go/bin:/usr/bin:/bin

# Security
NoNewPrivileges=yes
ProtectHome=yes
ProtectSystem=strict
ReadWritePaths=/opt/attendance

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable attendance
    systemctl start attendance

    echo "Application service started"
else
    echo ""
    echo "=== Skipping application deployment (GIT_REPO not set) ==="
    echo "You can deploy manually after startup completes."
fi

# Configure Nginx
echo ""
echo "=== Configuring Nginx ==="
cat > /etc/nginx/sites-available/attendance <<EOF
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name $DOMAIN;

    root /var/www/attendance;
    index index.html;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Frontend (static files)
    location / {
        try_files \$uri \$uri/ /index.html;
    }

    # Backend API proxy
    location /api {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 300;
        proxy_connect_timeout 300;
        proxy_send_timeout 300;
    }

    # SSE endpoint for real-time updates
    location /api/sse {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Connection '';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding off;
        proxy_read_timeout 86400;
    }

    # Health check
    location /health {
        proxy_pass http://localhost:8080;
        access_log off;
    }
}
EOF

rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/attendance /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx

# Create placeholder if no app deployed
if [ ! -f /var/www/attendance/index.html ]; then
    cat > /var/www/attendance/index.html <<EOF
<!DOCTYPE html>
<html>
<head>
    <title>236SA Attendance System</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; background: #f5f5f5; }
        .container { max-width: 600px; margin: 0 auto; background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; }
        p { color: #666; }
        code { background: #e8e8e8; padding: 2px 8px; border-radius: 4px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>236SA Attendance System</h1>
        <p>Server is ready. Application deployment pending.</p>
        <p>Run the following to complete setup:</p>
        <p><code>sudo certbot --nginx -d $DOMAIN</code></p>
    </div>
</body>
</html>
EOF
    chown www-data:www-data /var/www/attendance/index.html
fi

# Display system information
echo ""
echo "=== System setup completed at $(date) ==="
echo ""
echo "Installed versions:"
echo "  - PostgreSQL: $(psql --version)"
echo "  - Go: $(go version)"
echo "  - Node.js: $(node --version)"
echo "  - Nginx: $(nginx -v 2>&1)"
echo ""
echo "Memory usage:"
free -h
echo ""
echo "Disk usage:"
df -h /
echo ""
echo "=== Setup complete ==="
echo ""
echo "NEXT STEPS:"
echo "  1. Set up SSL: sudo certbot --nginx -d $DOMAIN"
echo "  2. Check application: sudo systemctl status attendance"
echo "  3. View logs: sudo journalctl -u attendance -f"
echo ""
