# ---------------------------------------------------------------------------------------------------------------------
# PIN TERRAFORM VERSION TO >= 0.12
# ---------------------------------------------------------------------------------------------------------------------

terraform {
  # This module is now only being tested with Terraform 0.13.x. However, to make upgrading easier, we are setting
  # 0.12.26 as the minimum version, as that version added support for required_providers with source URLs, making it
  # forwards compatible with 0.13.x code.
  required_version = ">= 0.12.26"
}

# ---------------------------------------------------------------------------------------------------------------------
# CREATE A FLOATING ENI WITH SECURITY GROUP
# ---------------------------------------------------------------------------------------------------------------------

resource "aws_security_group" "allow_tls" {
  name        = "${var.prefix}-kubergrunt-test-sg"
  description = "Allow TLS inbound traffic"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "TLS from VPC"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [data.aws_vpc.default.cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.prefix}-kubergrunt-test-sg"
  }
}

resource "aws_network_interface" "allow_tls" {
  description     = "Test network interface that should not be attached"
  subnet_id       = local.first_subnet
  security_groups = [aws_security_group.allow_tls.id]

  tags = {
    Name = "${var.prefix}-kubergrunt-test-network-interface"
  }
}

locals {
  first_subnet = sort(tolist(data.aws_subnets.default.ids))[0]
}


# ---------------------------------------------------------------------------------------------------------------------
# DATA SOURCES
# ---------------------------------------------------------------------------------------------------------------------

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  vpc_id = data.aws_vpc.default.id
}
