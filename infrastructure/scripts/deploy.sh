#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/config.sh"
validate_config

echo "=== Deploying 236SA Attendance System (EC2) ==="
echo "Region: $AWS_REGION"
echo "Instance Type: $INSTANCE_TYPE (~\$0.021/hour)"
echo "Domain: $DOMAIN"
echo ""

# Step 1: Create SSH key pair if not exists
echo "Step 1: Checking SSH key pair..."
if ! aws ec2 describe-key-pairs --key-names "$KEY_NAME" --region "$AWS_REGION" &>/dev/null; then
    echo "  Creating SSH key pair..."
    aws ec2 create-key-pair \
        --key-name "$KEY_NAME" \
        --region "$AWS_REGION" \
        --query 'KeyMaterial' \
        --output text > "$SCRIPT_DIR/${KEY_NAME}.pem"
    chmod 600 "$SCRIPT_DIR/${KEY_NAME}.pem"
    echo "  Key saved to: $SCRIPT_DIR/${KEY_NAME}.pem"
else
    echo "  SSH key pair already exists"
    if [ ! -f "$SCRIPT_DIR/${KEY_NAME}.pem" ]; then
        echo "  WARNING: Key exists in AWS but local .pem file not found!"
        echo "  You may need to delete the key and recreate: aws ec2 delete-key-pair --key-name $KEY_NAME --region $AWS_REGION"
    fi
fi

# Step 2: Create security group if not exists
echo ""
echo "Step 2: Checking security group..."
SG_ID=$(get_security_group_id)
if [ -z "$SG_ID" ] || [ "$SG_ID" == "None" ]; then
    echo "  Creating security group..."
    SG_ID=$(aws ec2 create-security-group \
        --group-name "$SECURITY_GROUP_NAME" \
        --description "Security group for 236SA Attendance System" \
        --region "$AWS_REGION" \
        --query 'GroupId' \
        --output text)

    # Add inbound rules
    echo "  Adding firewall rules..."
    aws ec2 authorize-security-group-ingress \
        --group-id "$SG_ID" \
        --region "$AWS_REGION" \
        --ip-permissions \
            IpProtocol=tcp,FromPort=22,ToPort=22,IpRanges='[{CidrIp=0.0.0.0/0,Description="SSH"}]' \
            IpProtocol=tcp,FromPort=80,ToPort=80,IpRanges='[{CidrIp=0.0.0.0/0,Description="HTTP"}]' \
            IpProtocol=tcp,FromPort=443,ToPort=443,IpRanges='[{CidrIp=0.0.0.0/0,Description="HTTPS"}]'

    echo "  Security group created: $SG_ID"
else
    echo "  Security group already exists: $SG_ID"
fi

# Step 3: Allocate Elastic IP if not exists
echo ""
echo "Step 3: Checking Elastic IP..."
EIP_ALLOC_ID=$(get_eip_allocation_id)
if [ -z "$EIP_ALLOC_ID" ] || [ "$EIP_ALLOC_ID" == "None" ]; then
    echo "  Allocating Elastic IP..."
    EIP_ALLOC_ID=$(aws ec2 allocate-address \
        --domain vpc \
        --region "$AWS_REGION" \
        --query 'AllocationId' \
        --output text)

    # Tag the Elastic IP
    aws ec2 create-tags \
        --resources "$EIP_ALLOC_ID" \
        --tags "Key=Name,Value=$EIP_NAME" \
        --region "$AWS_REGION"

    echo "  Elastic IP allocated: $EIP_ALLOC_ID"
else
    echo "  Elastic IP already allocated: $EIP_ALLOC_ID"
fi

ELASTIC_IP=$(aws ec2 describe-addresses \
    --allocation-ids "$EIP_ALLOC_ID" \
    --region "$AWS_REGION" \
    --query 'Addresses[0].PublicIp' \
    --output text)
echo "  Public IP: $ELASTIC_IP"

# Step 4: Create EC2 instance if not exists
echo ""
echo "Step 4: Checking EC2 instance..."
INSTANCE_ID=$(get_instance_id)
if [ -z "$INSTANCE_ID" ] || [ "$INSTANCE_ID" == "None" ]; then
    echo "  Launching EC2 instance..."

    # Prepare user-data with environment variables
    USER_DATA=$(cat "$SCRIPT_DIR/startup.sh" | sed \
        -e "s|\${GIT_REPO:-}|$GIT_REPO|g" \
        -e "s|\${GIT_BRANCH:-main}|$GIT_BRANCH|g" \
        -e "s|\${DOMAIN:-236sa.one}|$DOMAIN|g" \
        -e "s|\${DB_NAME:-attendance}|$DB_NAME|g" \
        -e "s|\${DB_USER:-attendance}|$DB_USER|g" \
        -e "s|\${DB_PASSWORD:-}|$DB_PASSWORD|g")

    INSTANCE_ID=$(aws ec2 run-instances \
        --image-id "$AMI_ID" \
        --instance-type "$INSTANCE_TYPE" \
        --key-name "$KEY_NAME" \
        --security-group-ids "$SG_ID" \
        --block-device-mappings "DeviceName=/dev/sda1,Ebs={VolumeSize=$VOLUME_SIZE,VolumeType=gp3,DeleteOnTermination=true}" \
        --user-data "$USER_DATA" \
        --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=$INSTANCE_NAME}]" \
        --region "$AWS_REGION" \
        --query 'Instances[0].InstanceId' \
        --output text)

    echo "  Instance launched: $INSTANCE_ID"
else
    echo "  Instance already exists: $INSTANCE_ID"
fi

# Step 5: Wait for instance to be running
echo ""
echo "Step 5: Waiting for instance to start..."
aws ec2 wait instance-running \
    --instance-ids "$INSTANCE_ID" \
    --region "$AWS_REGION"
echo "  Instance is running"

# Step 6: Associate Elastic IP
echo ""
echo "Step 6: Associating Elastic IP..."
ASSOCIATION_ID=$(aws ec2 describe-addresses \
    --allocation-ids "$EIP_ALLOC_ID" \
    --region "$AWS_REGION" \
    --query 'Addresses[0].AssociationId' \
    --output text 2>/dev/null)

if [ -z "$ASSOCIATION_ID" ] || [ "$ASSOCIATION_ID" == "None" ]; then
    aws ec2 associate-address \
        --instance-id "$INSTANCE_ID" \
        --allocation-id "$EIP_ALLOC_ID" \
        --region "$AWS_REGION" > /dev/null
    echo "  Elastic IP associated"
else
    echo "  Elastic IP already associated"
fi

# Step 7: Update DNS
echo ""
echo "Step 7: Updating DNS records..."
aws route53 change-resource-record-sets \
    --hosted-zone-id "$HOSTED_ZONE_ID" \
    --change-batch "{
        \"Changes\": [{
            \"Action\": \"UPSERT\",
            \"ResourceRecordSet\": {
                \"Name\": \"$DOMAIN\",
                \"Type\": \"A\",
                \"TTL\": 300,
                \"ResourceRecords\": [{\"Value\": \"$ELASTIC_IP\"}]
            }
        }]
    }" > /dev/null
echo "  DNS A record: $DOMAIN -> $ELASTIC_IP"

# Summary
echo ""
echo "=========================================="
echo "  DEPLOYMENT COMPLETE"
echo "=========================================="
echo ""
echo "Instance ID:   $INSTANCE_ID"
echo "Instance Type: $INSTANCE_TYPE"
echo "IP Address:    $ELASTIC_IP"
echo "Domain:        https://$DOMAIN"
echo "SSH Key:       $SCRIPT_DIR/${KEY_NAME}.pem"
echo ""
echo "Hourly Cost:   ~\$0.021/hour (~\$0.50/day)"
echo ""
echo "SSH Command:"
echo "  ssh -i $SCRIPT_DIR/${KEY_NAME}.pem ubuntu@$ELASTIC_IP"
echo ""
echo "NEXT STEPS:"
echo "  1. Wait ~10-15 minutes for startup script to complete"
echo "  2. Monitor progress:"
echo "     ssh -i $SCRIPT_DIR/${KEY_NAME}.pem ubuntu@$ELASTIC_IP 'tail -f /var/log/startup-script.log'"
echo "  3. After startup completes, set up SSL:"
echo "     ssh -i $SCRIPT_DIR/${KEY_NAME}.pem ubuntu@$ELASTIC_IP 'sudo certbot --nginx -d $DOMAIN'"
echo ""
echo "IMPORTANT: Remember to run 'make infra-destroy' after the call-up to stop billing!"
echo ""
