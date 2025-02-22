locals {
  default_admins = tolist([
    "arn:aws:iam::478845996840:user/github",
  ])
  admin_policy = {
    admin = {
      policy_arn = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
      access_scope = {
        type = "cluster",
      }
    }
  }
  cluster_addons = {
    coredns = {
      most_recent = true,
    },
    kube-proxy = {
      most_recent = true,
    },
    eks-pod-identity-agent = {
      most_recent = true,
    },
  }
}

