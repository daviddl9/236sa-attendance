#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/config.sh"

# Get current instance ID
INSTANCE_ID=$(get_instance_id)
EIP_ALLOC_ID=$(get_eip_allocation_id)

echo "=== Tearing Down 236SA Attendance Infrastructure (EC2) ==="
echo ""
echo "This will destroy:"
echo "  - EC2 instance: $INSTANCE_NAME ${INSTANCE_ID:+($INSTANCE_ID)}"
echo "  - Elastic IP: $EIP_NAME"
echo ""
echo "This will KEEP:"
echo "  - Domain registration: $DOMAIN"
echo "  - Route 53 hosted zone"
echo "  - Security group: $SECURITY_GROUP_NAME"
echo "  - SSH key pair: $KEY_NAME"
echo ""

# Confirm destruction
read -p "Are you sure you want to proceed? (yes/no): " CONFIRM
if [ "$CONFIRM" != "yes" ]; then
    echo "Aborted."
    exit 0
fi

echo ""

# Step 1: Terminate EC2 instance
echo "Step 1: Terminating EC2 instance..."
if [ -n "$INSTANCE_ID" ] && [ "$INSTANCE_ID" != "None" ]; then
    aws ec2 terminate-instances \
        --instance-ids "$INSTANCE_ID" \
        --region "$AWS_REGION" > /dev/null
    echo "  Instance termination initiated: $INSTANCE_ID"

    # Wait for instance to terminate
    echo "  Waiting for instance to terminate..."
    aws ec2 wait instance-terminated \
        --instance-ids "$INSTANCE_ID" \
        --region "$AWS_REGION"
    echo "  Instance terminated"
else
    echo "  No running instance found"
fi

# Step 2: Release Elastic IP
echo ""
echo "Step 2: Releasing Elastic IP..."
if [ -n "$EIP_ALLOC_ID" ] && [ "$EIP_ALLOC_ID" != "None" ]; then
    # Disassociate first if associated
    ASSOCIATION_ID=$(aws ec2 describe-addresses \
        --allocation-ids "$EIP_ALLOC_ID" \
        --region "$AWS_REGION" \
        --query 'Addresses[0].AssociationId' \
        --output text 2>/dev/null)

    if [ -n "$ASSOCIATION_ID" ] && [ "$ASSOCIATION_ID" != "None" ]; then
        aws ec2 disassociate-address \
            --association-id "$ASSOCIATION_ID" \
            --region "$AWS_REGION" 2>/dev/null || true
        echo "  Elastic IP disassociated"
    fi

    # Release the Elastic IP
    aws ec2 release-address \
        --allocation-id "$EIP_ALLOC_ID" \
        --region "$AWS_REGION"
    echo "  Elastic IP released"
else
    echo "  No Elastic IP found"
fi

# Summary
echo ""
echo "=========================================="
echo "  TEARDOWN COMPLETE"
echo "=========================================="
echo ""
echo "Resources destroyed:"
echo "  - EC2 instance (stopped billing)"
echo "  - Elastic IP (stopped billing)"
echo "  - EBS volume (deleted with instance)"
echo ""
echo "Resources preserved (for next deployment):"
echo "  - Domain: $DOMAIN (manually renew if needed)"
echo "  - Route 53 hosted zone (~\$0.50/month)"
echo "  - Security group: $SECURITY_GROUP_NAME"
echo "  - SSH key pair: $KEY_NAME"
echo ""
echo "Current billing: \$0/hour (only Route 53 hosted zone charge remains)"
echo ""
echo "To redeploy, run: make infra-deploy"
echo ""
