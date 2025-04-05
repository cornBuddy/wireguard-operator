---
title: Install
nav_order: 2
---

# Install

The easiest way to install operator is via kubectl

```bash
kubectl apply -f \
    https://github.com/cornbuddy/wireguard-operator/raw/refs/heads/main/src/config/manifest.yml
```

This command will install the latest stable version of the operator to your
kubernetes cluster
