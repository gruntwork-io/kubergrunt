output "security_group_id" {
  value = aws_security_group.allow_tls.id
}

output "eni_id" {
  value = aws_network_interface.allow_tls.id
}

output "subnet_id" {
  value = local.first_subnet
}
