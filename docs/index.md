---
title: Home
layout: home
nav_order: 0
---

# Wireguard operator
{: .no_toc }

This product is created to provision wireguard peers inside k8s cluster using
CRDs. It was tested well in EKS cluster with cilium CNI running in overlay mode.

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

## Install

The easiest way to install operator is via kubectl:

```bash
kubectl apply -f \
    https://github.com/cornbuddy/wireguard-operator/raw/refs/heads/main/src/config/manifest.yml
```

This command will install the latest stable version of the operator to your
kubernetes cluster.
