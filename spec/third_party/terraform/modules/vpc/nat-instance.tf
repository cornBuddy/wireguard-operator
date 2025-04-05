resource "aws_security_group" "nat_instance" {
  name        = "${var.name}-nat-instance-sg"
  vpc_id      = aws_vpc.this.id
  description = "Security group for NAT instance ${var.name}"
  tags        = var.extra_tags
}

resource "aws_security_group_rule" "nat_instance_egress" {
  security_group_id = aws_security_group.nat_instance.id
  type              = "egress"
  cidr_blocks       = ["0.0.0.0/0"]
  from_port         = 0
  to_port           = 65535
  protocol          = "all"
}

resource "aws_security_group_rule" "nat_instance_ingress" {
  security_group_id = aws_security_group.nat_instance.id
  type              = "ingress"
  cidr_blocks       = [aws_vpc.this.cidr_block]
  from_port         = 0
  to_port           = 65535
  protocol          = "all"
}

resource "aws_network_interface" "nat_instance" {
  for_each = var.use_nat_instance ? local.public_subnets : {}

  subnet_id         = aws_subnet.public[each.key].id
  source_dest_check = false
  security_groups   = [aws_security_group.nat_instance.id]
  description       = "ENI for NAT instance ${var.name}"
  tags              = var.extra_tags
}

resource "aws_route_table" "private_nat_instance" {
  for_each = var.use_nat_instance ? local.public_subnets : {}

  vpc_id = aws_vpc.this.id
  route {
    cidr_block           = "0.0.0.0/0"
    network_interface_id = aws_network_interface.nat_instance[each.key].id
  }
  tags = var.extra_tags
}

resource "aws_route_table_association" "private_nat_instance" {
  for_each = var.use_nat_instance ? local.private_subnets : {}

  subnet_id      = aws_subnet.private[each.key].id
  route_table_id = aws_route_table.private_nat_instance[each.key].id
}

module "role" {
  source = "../../modules/iam-role"

  name             = "${var.name}-nat-instance"
  managed_policies = ["AmazonSSMManagedInstanceCore"]
  principal = {
    Service = "ec2.amazonaws.com",
  }
  extra_tags = var.extra_tags
}

resource "aws_iam_instance_profile" "this" {
  name = "${var.name}-nat-instance"
  role = module.role.name
  tags = var.extra_tags
}

resource "aws_launch_template" "nat_instance" {
  for_each = var.use_nat_instance ? local.public_subnets : {}

  name          = "${var.name}-${each.key}-nat-instance-lt"
  instance_type = var.nat_instance_types[0]
  image_id      = data.aws_ami.nat_instance.id
  description   = "Launch template for NAT instance ${var.name}"
  network_interfaces {
    device_index         = 0
    network_interface_id = aws_network_interface.nat_instance[each.key].id
  }
  block_device_mappings {
    device_name = data.aws_ami.nat_instance.root_device_name
    ebs {
      volume_type = "gp3"
      volume_size = 8
    }
  }
  iam_instance_profile {
    name = aws_iam_instance_profile.this.name
  }
  tag_specifications {
    resource_type = "instance"
    tags          = merge(local.all_tags, { Name = "${var.name}-nat-instance" })
  }
  tags = var.extra_tags
}

resource "aws_autoscaling_group" "nat_instance" {
  for_each = var.use_nat_instance ? local.public_subnets : {}

  name                      = "${var.name}-${each.key}-nat-instance-asg"
  default_cooldown          = 10
  health_check_grace_period = 60
  desired_capacity          = 1
  min_size                  = 1
  max_size                  = 2
  availability_zones        = [aws_subnet.public[each.key].availability_zone]
  mixed_instances_policy {
    instances_distribution {
      on_demand_base_capacity                  = 1
      on_demand_percentage_above_base_capacity = 100
    }
    launch_template {
      launch_template_specification {
        launch_template_id = aws_launch_template.nat_instance[each.key].id
        version            = "$Latest"
      }
      dynamic "override" {
        for_each = var.nat_instance_types
        content {
          instance_type = override.value
        }
      }
    }
  }
  lifecycle {
    create_before_destroy = true
  }
  dynamic "tag" {
    for_each = local.all_tags_asg

    content {
      key                 = tag.value["key"]
      value               = tag.value["value"]
      propagate_at_launch = tag.value["propagate_at_launch"]
    }
  }
}
