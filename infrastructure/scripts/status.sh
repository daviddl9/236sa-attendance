#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/config.sh"

echo "=== 236SA Attendance Infrastructure Status (EC2) ==="
echo ""

# Get IDs
INSTANCE_ID=$(get_instance_id)
SG_ID=$(get_security_group_id)
EIP_ALLOC_ID=$(get_eip_allocation_id)

# Instance status
echo "EC2 INSTANCE"
echo "----------------------------------------"
if [ -n "$INSTANCE_ID" ] && [ "$INSTANCE_ID" != "None" ]; then
    INSTANCE_INFO=$(aws ec2 describe-instances \
        --instance-ids "$INSTANCE_ID" \
        --region "$AWS_REGION" \
        --query 'Reservations[0].Instances[0].{State:State.Name,Type:InstanceType,PublicIP:PublicIpAddress,PrivateIP:PrivateIpAddress,LaunchTime:LaunchTime}' \
        --output json 2>/dev/null)

    echo "$INSTANCE_INFO" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f\"  Instance ID:  $INSTANCE_ID\")
print(f\"  State:        {d['State']}\")
print(f\"  Type:         {d['Type']}\")
print(f\"  Public IP:    {d['PublicIP'] or 'None'}\")
print(f\"  Private IP:   {d['PrivateIP'] or 'None'}\")
print(f\"  Launch Time:  {d['LaunchTime']}\")
"
else
    echo "  Status: NOT DEPLOYED"
fi

# Elastic IP status
echo ""
echo "ELASTIC IP"
echo "----------------------------------------"
if [ -n "$EIP_ALLOC_ID" ] && [ "$EIP_ALLOC_ID" != "None" ]; then
    EIP_INFO=$(aws ec2 describe-addresses \
        --allocation-ids "$EIP_ALLOC_ID" \
        --region "$AWS_REGION" \
        --query 'Addresses[0].{IP:PublicIp,Associated:AssociationId,InstanceId:InstanceId}' \
        --output json 2>/dev/null)

    echo "$EIP_INFO" | python3 -c "
import sys, json
d = json.load(sys.stdin)
is_associated = d['Associated'] is not None
print(f\"  Allocation ID: $EIP_ALLOC_ID\")
print(f\"  IP Address:    {d['IP']}\")
print(f\"  Associated:    {is_associated}\")
if d['InstanceId']:
    print(f\"  Instance:      {d['InstanceId']}\")
"
else
    echo "  Status: NOT ALLOCATED"
fi

# Security Group status
echo ""
echo "SECURITY GROUP"
echo "----------------------------------------"
if [ -n "$SG_ID" ] && [ "$SG_ID" != "None" ]; then
    echo "  Name:     $SECURITY_GROUP_NAME"
    echo "  Group ID: $SG_ID"

    # List inbound rules
    RULES=$(aws ec2 describe-security-groups \
        --group-ids "$SG_ID" \
        --region "$AWS_REGION" \
        --query 'SecurityGroups[0].IpPermissions[*].[IpProtocol,FromPort,ToPort]' \
        --output text 2>/dev/null)

    echo "  Inbound Rules:"
    echo "$RULES" | while read -r proto from to; do
        [ -n "$proto" ] && echo "    - $proto: $from-$to"
    done
else
    echo "  Status: NOT CREATED"
fi

# DNS status
echo ""
echo "DNS RECORDS"
echo "----------------------------------------"
if [ -n "$HOSTED_ZONE_ID" ]; then
    DNS_RECORD=$(aws route53 list-resource-record-sets \
        --hosted-zone-id "$HOSTED_ZONE_ID" \
        --query "ResourceRecordSets[?Name=='${DOMAIN}.']" \
        --output json 2>/dev/null)

    if [ -n "$DNS_RECORD" ] && [ "$DNS_RECORD" != "[]" ]; then
        echo "$DNS_RECORD" | python3 -c "
import sys, json
records = json.load(sys.stdin)
for r in records:
    rtype = r['Type']
    name = r['Name'].rstrip('.')
    if 'ResourceRecords' in r:
        values = ', '.join([v['Value'] for v in r['ResourceRecords']])
    elif 'AliasTarget' in r:
        values = r['AliasTarget']['DNSName']
    else:
        values = 'N/A'
    ttl = r.get('TTL', 'Alias')
    print(f\"  {rtype:6} {name:30} -> {values} (TTL: {ttl})\")
"
    else
        echo "  No A record found for $DOMAIN"
    fi
else
    echo "  HOSTED_ZONE_ID not configured in config.sh"
fi

# SSH Key status
echo ""
echo "SSH KEY PAIR"
echo "----------------------------------------"
KEY_EXISTS=$(aws ec2 describe-key-pairs \
    --key-names "$KEY_NAME" \
    --region "$AWS_REGION" \
    --query 'KeyPairs[0].KeyName' \
    --output text 2>/dev/null)

if [ -n "$KEY_EXISTS" ] && [ "$KEY_EXISTS" != "None" ]; then
    echo "  AWS Key:    $KEY_NAME (exists)"
    if [ -f "$SCRIPT_DIR/${KEY_NAME}.pem" ]; then
        echo "  Local Key:  $SCRIPT_DIR/${KEY_NAME}.pem (exists)"
    else
        echo "  Local Key:  NOT FOUND (key may need to be recreated)"
    fi
else
    echo "  Status: NOT CREATED"
fi

# Cost estimate
echo ""
echo "COST ESTIMATE"
echo "----------------------------------------"
if [ -n "$INSTANCE_ID" ] && [ "$INSTANCE_ID" != "None" ]; then
    STATE=$(aws ec2 describe-instances \
        --instance-ids "$INSTANCE_ID" \
        --region "$AWS_REGION" \
        --query 'Reservations[0].Instances[0].State.Name' \
        --output text 2>/dev/null)

    if [ "$STATE" == "running" ]; then
        echo "  EC2 ($INSTANCE_TYPE):      ~\$0.021/hour (~\$0.50/day)"
        echo "  EBS Storage (${VOLUME_SIZE}GB):    ~\$0.003/hour"
        echo "  Route 53 hosted zone:   ~\$0.50/month"
        echo "  --------------------------------"
        echo "  Total:                  ~\$0.024/hour while running"
    else
        echo "  Instance state: $STATE (not billing for compute)"
        echo "  Route 53 hosted zone:   ~\$0.50/month"
    fi
else
    echo "  No active resources (infrastructure not deployed)"
    if [ -n "$HOSTED_ZONE_ID" ]; then
        echo "  Route 53 hosted zone:   ~\$0.50/month (if exists)"
    fi
fi

echo ""
