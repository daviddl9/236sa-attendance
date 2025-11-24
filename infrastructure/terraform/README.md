# Attendance System - AWS Lightsail Terraform Infrastructure

This directory contains Terraform configuration for deploying the Attendance System to AWS Lightsail in Singapore (ap-southeast-1).

## Architecture

- **Platform**: AWS Lightsail
- **Region**: Singapore (ap-southeast-1)
- **Instance**: $12/month (2 vCPU, 2GB RAM, 60GB SSD)
- **OS**: Ubuntu 24.04 LTS
- **Services**: PostgreSQL 17, Go backend, React frontend, Nginx

## Cost Estimate

**$12/month fixed price** includes:
- Compute instance
- 60GB SSD storage
- 3TB data transfer
- Static IP address

## Prerequisites

1. **AWS Account** with Lightsail access
2. **AWS CLI** configured with credentials
3. **Terraform** v1.0 or later installed

### Install Terraform (macOS)
```bash
brew install terraform
```

### Configure AWS Credentials
```bash
aws configure
# Enter your AWS Access Key ID, Secret Access Key, and region (ap-southeast-1)
```

## Quick Start

### 1. Initialize Terraform
```bash
cd infrastructure/terraform
terraform init
```

### 2. Review the Plan
```bash
terraform plan
```

### 3. Deploy Infrastructure
```bash
terraform apply
```

Type `yes` when prompted to create the resources.

### 4. Wait for Setup to Complete
The startup script takes ~10-15 minutes to:
- Install PostgreSQL 17, Go 1.23, Node.js 22, Nginx
- Configure PostgreSQL for 2GB RAM
- Create 4GB swap file
- Setup basic Nginx configuration

SSH to the server and monitor progress:
```bash
ssh -i attendance-key.pem ubuntu@<STATIC_IP>
tail -f /var/log/startup-script.log
```

### 5. Deploy Application with Ansible
After infrastructure is ready, deploy the application:
```bash
cd ../ansible

# Create inventory file (Terraform outputs will show the command)
cat > inventory.ini <<EOF
[attendance]
<STATIC_IP> ansible_user=ubuntu ansible_ssh_private_key_file=../terraform/attendance-key.pem ansible_python_interpreter=/usr/bin/python3
EOF

# Run Ansible playbook
ansible-playbook -i inventory.ini playbook.yml
```

## Files Description

- **main.tf**: Terraform provider configuration
- **variables.tf**: Input variables (region, instance size, etc.)
- **key_pair.tf**: SSH key generation
- **lightsail.tf**: Lightsail instance and static IP
- **firewall.tf**: Port configuration (22, 80, 443)
- **startup_script.sh**: System setup script
- **outputs.tf**: Connection information
- **inventory.tpl**: Ansible inventory template
- **.gitignore**: Excludes sensitive files

## Configuration

### Change Instance Size

Edit `variables.tf` to use a different plan:

```hcl
variable "bundle_id" {
  default     = "small_3_0"  # $20/month: 2 vCPU, 4GB RAM, 80GB SSD
}
```

Available bundles:
- `micro_3_0`: $12/month (2 vCPU, 2GB RAM, 60GB SSD)
- `small_3_0`: $20/month (2 vCPU, 4GB RAM, 80GB SSD)
- `medium_3_0`: $40/month (2 vCPU, 8GB RAM, 160GB SSD)

## Outputs

After `terraform apply`, you'll see:
- **static_ip**: Public IP address
- **ssh_command**: Command to SSH to the server
- **http_url**: URL to access the application
- **next_steps**: Detailed instructions for deployment

## Accessing the Server

### SSH Access
```bash
ssh -i attendance-key.pem ubuntu@<STATIC_IP>
```

### Check Services
```bash
# PostgreSQL
sudo systemctl status postgresql
sudo -u postgres psql

# Nginx
sudo systemctl status nginx
sudo nginx -t

# View setup logs
tail -f /var/log/startup-script.log
```

## Setup SSL Certificate

After configuring your domain DNS:

```bash
ssh -i attendance-key.pem ubuntu@<STATIC_IP>
sudo certbot --nginx -d yourdomain.com
```

## Destroying Infrastructure

To delete all resources:

```bash
terraform destroy
```

Type `yes` to confirm deletion.

## Troubleshooting

### Setup Script Not Completing
Check the setup log:
```bash
ssh -i attendance-key.pem ubuntu@<STATIC_IP>
tail -f /var/log/startup-script.log
```

### PostgreSQL Not Starting
Check PostgreSQL logs:
```bash
sudo journalctl -u postgresql -f
```

### Out of Memory
The system has 4GB swap as a buffer. Check memory usage:
```bash
free -h
htop
```

If consistently hitting memory limits, upgrade to the $20/month plan (4GB RAM).

## Monitoring

### Memory Usage
```bash
free -h
watch -n 1 free -h
```

### Disk Usage
```bash
df -h
du -sh /var/lib/postgresql
```

### PostgreSQL Performance
```bash
sudo -u postgres psql
SELECT * FROM pg_stat_activity;
```

## Scaling Up

To upgrade to a larger instance:

1. Update `variables.tf`:
   ```hcl
   variable "bundle_id" {
     default = "small_3_0"  # 4GB RAM
   }
   ```

2. Apply changes:
   ```bash
   terraform apply
   ```

Note: This will cause ~5 minutes of downtime.

## Security Notes

- SSH key (`attendance-key.pem`) is auto-generated and excluded from git
- Firewall allows ports 22, 80, 443 only
- PostgreSQL only accessible from localhost
- Use Ansible vault for secrets management

## Support

For issues or questions, refer to the main project documentation.
