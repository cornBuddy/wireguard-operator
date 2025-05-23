module "vpc" {
  source = "../../modules/vpc"

  name     = var.name
  vpc_cidr = var.vpc_cidr
}

module "export_kubeconfig" {
  source = "../../modules/eks-export-kubeconfig"

  cluster_name = module.eks.cluster_name
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.20"

  cluster_name    = var.name
  cluster_addons  = local.cluster_addons
  cluster_version = "1.31"

  vpc_id                         = module.vpc.id
  subnet_ids                     = module.vpc.private_subnets
  control_plane_subnet_ids       = module.vpc.private_subnets
  cluster_endpoint_public_access = true

  cluster_encryption_config   = {}
  create_cloudwatch_log_group = false
  create_kms_key              = false

  eks_managed_node_group_defaults = {
    desired_size      = 2,
    disk_size         = 8,
    enable_monitoring = false,
    capacity_type     = "SPOT",
    ami_type          = "BOTTLEROCKET_ARM_64",
    instance_types    = ["t4g.medium", "m6g.medium", "m7g.medium"],
    metadata_options = {
      http_put_response_hop_limit = 3,
    }
    vpc_security_group_ids = [module.vpc.default_security_group]
    iam_role_additional_policies = {
      for key, value in data.aws_iam_policy.this :
      key => value.arn
    },
    bootstrap_extra_args = <<-EOT
    [settings.kubernetes]
    "allowed-unsafe-sysctls" = ["net.ipv4.*"]
    EOT
  }
  eks_managed_node_groups = { "${var.name}" = {} }

  enable_cluster_creator_admin_permissions = true
  cluster_additional_security_group_ids    = [module.vpc.default_security_group]
  node_security_group_additional_rules = {
    # cilium uses icmp to check connectivity, and icmp is not allowed by default
    icmp = {
      protocol    = "icmp",
      from_port   = -1,
      to_port     = -1,
      type        = "ingress",
      self        = true,
      description = "allow icmp traffic from within cluster",
    },
    # cilium uses this port for vxlan communication
    vxlan = {
      protocol    = "UDP",
      from_port   = 8472,
      to_port     = 8472,
      type        = "ingress",
      self        = true,
      description = "allow vxlan traffic from within cluster",
    },
  }
}
