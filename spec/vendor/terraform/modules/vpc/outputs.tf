output "id" {
  value = aws_vpc.this.id
}

output "public_subnets" {
  value = [for _, v in aws_subnet.public : v.id]
}

output "private_subnets" {
  value = [for _, v in aws_subnet.private : v.id]
}

output "default_security_group" {
  value = aws_vpc.this.default_security_group_id
}
