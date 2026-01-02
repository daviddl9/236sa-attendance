#!/bin/bash
# Shared configuration for all infrastructure scripts
# Update these values after setting up your AWS account and domain

# AWS Configuration
export AWS_REGION="ap-southeast-1"
export AVAILABILITY_ZONE="${AWS_REGION}a"

# EC2 Instance Configuration
export INSTANCE_NAME="attendance-server"
export INSTANCE_TYPE="t3.small"  # 2 vCPU, 2GB RAM - ~$0.021/hour
export KEY_NAME="attendance-key"
export SECURITY_GROUP_NAME="attendance-sg"
export VOLUME_SIZE="20"  # GB

# Ubuntu 24.04 LTS AMI for ap-southeast-1 (update if needed)
# Find latest: aws ec2 describe-images --owners 099720109477 --filters "Name=name,Values=ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*" --query 'Images | sort_by(@, &CreationDate) | [-1].ImageId' --output text
export AMI_ID="ami-0c1907b6d738188e5"

# Elastic IP Configuration
export EIP_NAME="attendance-eip"

# Domain Configuration
# Get this from Route 53 console after registering your domain
export DOMAIN="236sa.one"
export HOSTED_ZONE_ID=""  # e.g., "Z1234567890ABC"

# Application Configuration
export GIT_REPO=""  # e.g., "https://github.com/yourusername/attendance.git"
export GIT_BRANCH="main"

# Database Configuration
export DB_NAME="attendance"
export DB_USER="attendance"
export DB_PASSWORD=""  # Set a secure password

# Validation
validate_config() {
    local errors=0

    if [ -z "$HOSTED_ZONE_ID" ]; then
        echo "ERROR: HOSTED_ZONE_ID is not set in config.sh"
        errors=$((errors + 1))
    fi

    if [ -z "$GIT_REPO" ]; then
        echo "ERROR: GIT_REPO is not set in config.sh"
        errors=$((errors + 1))
    fi

    if [ -z "$DB_PASSWORD" ]; then
        echo "ERROR: DB_PASSWORD is not set in config.sh"
        errors=$((errors + 1))
    fi

    if [ $errors -gt 0 ]; then
        echo ""
        echo "Please update infrastructure/scripts/config.sh with the required values."
        exit 1
    fi
}

# Helper to get instance ID by name tag
get_instance_id() {
    aws ec2 describe-instances \
        --region "$AWS_REGION" \
        --filters "Name=tag:Name,Values=$INSTANCE_NAME" "Name=instance-state-name,Values=pending,running,stopping,stopped" \
        --query 'Reservations[0].Instances[0].InstanceId' \
        --output text 2>/dev/null
}

# Helper to get security group ID
get_security_group_id() {
    aws ec2 describe-security-groups \
        --region "$AWS_REGION" \
        --filters "Name=group-name,Values=$SECURITY_GROUP_NAME" \
        --query 'SecurityGroups[0].GroupId' \
        --output text 2>/dev/null
}

# Helper to get Elastic IP allocation ID
get_eip_allocation_id() {
    aws ec2 describe-addresses \
        --region "$AWS_REGION" \
        --filters "Name=tag:Name,Values=$EIP_NAME" \
        --query 'Addresses[0].AllocationId' \
        --output text 2>/dev/null
}
