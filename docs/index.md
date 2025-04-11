---
title: Home
layout: home
nav_order: 0
---

# Wireguard operator
{: .no_toc }

Create wireguard peers inside k8s cluster using CRDs

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

## Prerequisites

This operator is well tested in EKS with cilium CNI driver. It should work in
any other k8s cluster, but it's not guaranteed. However, wireguard itself
requires the following
[unsafe sysctls](https://kubernetes.io/docs/tasks/administer-cluster/sysctl-cluster/#safe-and-unsafe-sysctls)
to be set to:
* `net.ipv4.ip_forward=1`
* `net.ipv4.conf.all.src_valid_mark=1`
* `net.ipv4.conf.all.rp_filter=0`
* `net.ipv4.conf.all.route_localnet=1`

## Install

The easiest way to install operator is via kubectl:

```bash
kubectl apply -f \
    https://github.com/cornbuddy/wireguard-operator/raw/refs/heads/main/src/config/manifest.yml
```

This command will install the latest stable version of the operator to your
kubernetes cluster.

## Usage

First of all, you need to create `Wireugard` and `WireguardPeer` resource pair.
`Wireguard` represents "server" of the network. It reconciles into deployment,
which you most likely want to expose

```yaml
---
apiVersion: vpn.ahova.com/v1alpha1
kind: Wireguard
metadata:
  name: vpn
spec:
  serviceType: LoadBalancer

---
apiVersion: vpn.ahova.com/v1alpha1
kind: WireguardPeer
metadata:
  name: peer
spec:
  wireguardRef: vpn
```

After resources are reconciled, you can fetch peer configuration and connect
to wireguard:

```bash
kubectl get secret -o json peer \
    | jq -r '.data.config' \
    | base64 -d > /etc/wireguard/wg0.conf
sudo wg-quick up wg0
```
