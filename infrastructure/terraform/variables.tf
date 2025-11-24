variable "environment" {
  description = "Environment name (dev, staging, prod)"
  type        = string
  default     = "prod"
}

variable "aws_region" {
  description = "AWS region for Lightsail"
  type        = string
  default     = "ap-southeast-1"
}

variable "availability_zone" {
  description = "Availability zone for Lightsail instance"
  type        = string
  default     = "ap-southeast-1a"
}

variable "bundle_id" {
  description = "Lightsail bundle ID (micro_3_0 = $12/month, small_3_0 = $20/month)"
  type        = string
  default     = "micro_3_0" # 2 vCPU, 2GB RAM, 60GB SSD - $12/month
}

variable "blueprint_id" {
  description = "Lightsail blueprint ID (OS image)"
  type        = string
  default     = "ubuntu_24_04"
}
