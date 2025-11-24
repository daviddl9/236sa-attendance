# Output important connection information

output "instance_name" {
  description = "Name of the Lightsail instance"
  value       = aws_lightsail_instance.attendance.name
}

output "instance_id" {
  description = "ID of the Lightsail instance"
  value       = aws_lightsail_instance.attendance.id
}

output "static_ip" {
  description = "Static IP address of the instance"
  value       = aws_lightsail_static_ip.attendance.ip_address
}

output "ssh_private_key_path" {
  description = "Path to SSH private key"
  value       = local_file.private_key.filename
}

output "ssh_command" {
  description = "SSH command to connect to the instance"
  value       = "ssh -i ${local_file.private_key.filename} ubuntu@${aws_lightsail_static_ip.attendance.ip_address}"
}

output "http_url" {
  description = "HTTP URL to access the application"
  value       = "http://${aws_lightsail_static_ip.attendance.ip_address}"
}

output "next_steps" {
  description = "Next steps after infrastructure is created"
  value       = <<-EOT

    ════════════════════════════════════════════════════════════════
    🎉 Infrastructure Created Successfully!
    ════════════════════════════════════════════════════════════════

    📍 Server IP: ${aws_lightsail_static_ip.attendance.ip_address}
    🔐 SSH Key: ${local_file.private_key.filename}

    Next Steps:

    1. SSH to the server (wait ~10 min for setup to complete):
       ssh -i ${local_file.private_key.filename} ubuntu@${aws_lightsail_static_ip.attendance.ip_address}

    2. Check setup progress:
       tail -f /var/log/startup-script.log

    3. Create Ansible inventory file:
       cd ../ansible
       cat > inventory.ini <<EOF
       [attendance]
       ${aws_lightsail_static_ip.attendance.ip_address} ansible_user=ubuntu ansible_ssh_private_key_file=../terraform/${local_file.private_key.filename} ansible_python_interpreter=/usr/bin/python3
       EOF

    4. Deploy application with Ansible:
       ansible-playbook -i inventory.ini playbook.yml

    5. Access the application:
       http://${aws_lightsail_static_ip.attendance.ip_address}

    6. Setup SSL (after DNS is configured):
       ssh -i ${local_file.private_key.filename} ubuntu@${aws_lightsail_static_ip.attendance.ip_address}
       sudo certbot --nginx -d yourdomain.com

    💰 Cost: ~$12/month

    ════════════════════════════════════════════════════════════════
  EOT
}
