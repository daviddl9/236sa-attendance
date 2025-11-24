# Generate SSH key pair for accessing Lightsail instance
resource "tls_private_key" "lightsail_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Create Lightsail key pair from generated public key
resource "aws_lightsail_key_pair" "attendance" {
  name       = "attendance-key-${var.environment}"
  public_key = tls_private_key.lightsail_key.public_key_openssh
}

# Save private key locally for SSH access
resource "local_file" "private_key" {
  content         = tls_private_key.lightsail_key.private_key_pem
  filename        = "${path.module}/attendance-key.pem"
  file_permission = "0600"
}
