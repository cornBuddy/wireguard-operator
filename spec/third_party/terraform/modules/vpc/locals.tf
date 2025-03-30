locals {
  azs       = slice(data.aws_availability_zones.this.names, 0, 3)
  all_subns = cidrsubnets(var.vpc_cidr, 4, 4, 4, 2, 2, 2)
  public_subnets = {
    for key, value in zipmap(local.azs, slice(local.all_subns, 0, 3)) :
    key => value
  }
  private_subnets = {
    for key, value in zipmap(local.azs, slice(local.all_subns, 3, 6)) :
    key => value
  }

  use_nat_gateway = !var.use_nat_instance
  all_tags        = merge(data.aws_default_tags.this.tags, var.extra_tags)
  all_tags_asg = [
    for k, v in merge(data.aws_default_tags.this.tags, var.extra_tags) : {
      key                 = k,
      value               = v,
      propagate_at_launch = true,
    }
  ]
}
