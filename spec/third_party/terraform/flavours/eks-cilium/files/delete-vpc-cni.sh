#!/usr/bin/env bash

set -v

delete_if_exists() {
    namespace="$1"
    resource="$2"
    name="$3"

    if kubectl -n "$namespace" get "$resource" "$name"; then
        kubectl -n "$namespace" delete "$resource" "$name"
    fi
}

# https://github.com/littlejo/cilium-eks-cookbook/blob/main/install-cilium-eks-helm.md#cilium-installation

delete_if_exists "kube-system" "daemonset" "aws-node"
delete_if_exists "kube-system" "configmap" "amazon-vpc-cni"
