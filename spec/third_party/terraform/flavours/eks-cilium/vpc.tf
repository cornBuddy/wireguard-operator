module "vpc" {
  source = "../../modules/vpc"

  name     = var.name
  vpc_cidr = var.vpc_cidr
}
