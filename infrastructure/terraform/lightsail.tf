# Lightsail instance for attendance application
resource "aws_lightsail_instance" "attendance" {
  name              = "attendance-${var.environment}"
  availability_zone = var.availability_zone
  blueprint_id      = var.blueprint_id
  bundle_id         = var.bundle_id
  key_pair_name     = aws_lightsail_key_pair.attendance.name
  user_data         = file("${path.module}/startup_script.sh")

  tags = {
    Environment = var.environment
    Project     = "attendance"
    ManagedBy   = "terraform"
  }
}

# Static IP for persistent addressing
resource "aws_lightsail_static_ip" "attendance" {
  name = "attendance-ip-${var.environment}"
}

# Attach static IP to instance
resource "aws_lightsail_static_ip_attachment" "attendance" {
  static_ip_name = aws_lightsail_static_ip.attendance.name
  instance_name  = aws_lightsail_instance.attendance.name
}
