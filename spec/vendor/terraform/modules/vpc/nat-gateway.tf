resource "aws_eip" "nat_ip" {
  for_each = local.use_nat_gateway ? local.public_subnets : {}

  vpc  = true
  tags = var.extra_tags
}

resource "aws_nat_gateway" "this" {
  for_each = local.use_nat_gateway ? local.public_subnets : {}

  subnet_id     = aws_subnet.public[each.key].id
  allocation_id = aws_eip.nat_ip[each.key].id
  tags          = var.extra_tags
}

resource "aws_route_table" "private_nat_gateway" {
  for_each = local.use_nat_gateway ? local.private_subnets : {}

  vpc_id = aws_vpc.this.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.this[each.key].id
  }
  tags = var.extra_tags
}

resource "aws_route_table_association" "private_nat_gateway" {
  for_each = local.use_nat_gateway ? local.private_subnets : {}

  subnet_id      = aws_subnet.private[each.key].id
  route_table_id = aws_route_table.private_nat_gateway[each.key].id
}
