locals {
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

