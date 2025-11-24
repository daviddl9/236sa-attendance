#!/bin/bash
set -e

# Log all output
exec > >(tee /var/log/startup-script.log)
exec 2>&1

echo "=== Starting system setup at $(date) ==="

# Update system packages
echo "Updating system packages..."
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get upgrade -y

# Install base dependencies
echo "Installing base dependencies..."
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
  unzip

# Install PostgreSQL 17
echo "Installing PostgreSQL 17..."
sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add -
apt-get update
apt-get install -y postgresql-17 postgresql-contrib-17

# Configure PostgreSQL for 2GB RAM
echo "Configuring PostgreSQL for low memory..."
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

# Restart PostgreSQL with new configuration
systemctl restart postgresql
systemctl enable postgresql

echo "PostgreSQL 17 installed and configured"

# Install Go 1.23
echo "Installing Go 1.23..."
wget https://go.dev/dl/go1.23.3.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.23.3.linux-amd64.tar.gz
rm go1.23.3.linux-amd64.tar.gz

# Add Go to PATH for all users
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/bash.bashrc
export PATH=$PATH:/usr/local/go/bin

echo "Go 1.23 installed"

# Install Node.js 22
echo "Installing Node.js 22..."
curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
apt-get install -y nodejs

echo "Node.js 22 installed: $(node --version)"

# Install Nginx
echo "Installing Nginx..."
apt-get install -y nginx
systemctl enable nginx

echo "Nginx installed"

# Install certbot for Let's Encrypt
echo "Installing certbot..."
apt-get install -y certbot python3-certbot-nginx

echo "Certbot installed"

# Create application user
echo "Creating application user..."
useradd -r -m -s /bin/bash -d /opt/attendance attendance

# Create application directories
mkdir -p /opt/attendance/{backend,frontend}
mkdir -p /var/www/attendance
chown -R attendance:attendance /opt/attendance
chown -R attendance:attendance /var/www/attendance

echo "Application directories created"

# Create 4GB swap file (safety buffer for 2GB RAM)
echo "Creating 4GB swap file..."
fallocate -l 4G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab

echo "Swap file created and enabled"

# Install Ansible for application deployment
echo "Installing Ansible..."
pip3 install ansible

echo "Ansible installed: $(ansible --version | head -n 1)"

# Create systemd service template directory
mkdir -p /etc/systemd/system

# Set up basic Nginx configuration
cat > /etc/nginx/sites-available/attendance <<'EOF'
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;

    root /var/www/attendance;
    index index.html;

    # Frontend
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Backend API proxy
    location /api {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }
}
EOF

# Enable Nginx site
rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/attendance /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx

echo "Nginx configured"

# Create placeholder index.html
cat > /var/www/attendance/index.html <<EOF
<!DOCTYPE html>
<html>
<head>
    <title>Attendance System</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
        h1 { color: #333; }
        p { color: #666; }
    </style>
</head>
<body>
    <h1>Attendance System</h1>
    <p>Server is ready. Deploy the application using Ansible.</p>
    <p>Server IP: <strong id="ip"></strong></p>
    <script>
        fetch('https://api.ipify.org?format=json')
            .then(r => r.json())
            .then(d => document.getElementById('ip').textContent = d.ip);
    </script>
</body>
</html>
EOF

chown -R www-data:www-data /var/www/attendance

# Display system information
echo "=== System setup completed at $(date) ==="
echo ""
echo "Installed versions:"
echo "- PostgreSQL: $(psql --version)"
echo "- Go: $(go version)"
echo "- Node.js: $(node --version)"
echo "- Nginx: $(nginx -v 2>&1)"
echo "- Ansible: $(ansible --version | head -n 1)"
echo ""
echo "Memory usage:"
free -h
echo ""
echo "Disk usage:"
df -h
echo ""
echo "=== Setup complete. Ready for Ansible deployment ==="
