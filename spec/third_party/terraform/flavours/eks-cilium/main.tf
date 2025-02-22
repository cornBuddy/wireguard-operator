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

  access_entries = merge({
    for arn in local.default_admins :
    "${split("/", arn)[1]}" => {
      principal_arn       = arn,
      type                = "STANDARD",
      policy_associations = local.admin_policy,
    }
  })

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
    bootstrap_extra_args    = <<-EOT
    [settings.kubernetes]
    "max-pods" = 50
    "allowed-unsafe-sysctls" = ["net.ipv4.*"]
    EOT
    pre_bootstrap_user_data = <<-EOT
      #!/bin/bash
      set -ex
      sudo yum install amazon-ssm-agent -y
      sudo systemctl enable amazon-ssm-agent
      sudo systemctl start amazon-ssm-agent
    EOT
  }
  eks_managed_node_groups = { "${var.name}-infra" = {} }

  cluster_additional_security_group_ids = [module.vpc.default_security_group]
  cluster_security_group_additional_rules = {
    # metrics server is run in host mode on that port
    metrics_server = {
      protocol    = "TCP",
      from_port   = 10251,
      to_port     = 10251,
      type        = "ingress",
      self        = true,
      description = "allow metrics server traffic",
    },
  }
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
    # https://docs.cilium.io/en/stable/network/concepts/routing/#requirements-on-the-network
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
