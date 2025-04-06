---
title: Examples
nav_order: 3
---

# Examples

Full list of examples is available in the
[samples directory](https://github.com/cornbuddy/wireguard-operator/tree/main/src/config/samples)

Very basic example of wireguard server and wireguard peer
```yaml
---
apiVersion: vpn.ahova.com/v1alpha1
kind: Wireguard
metadata:
  name: default-wg
spec:
  address: 192.168.1.1/24

---
apiVersion: vpn.ahova.com/v1alpha1
kind: WireguardPeer
metadata:
  name: default-peer
spec:
  wireguardRef: default-wg
  address: 192.168.1.2/32
```

Wireguard exporter running in sidecar mode
```yaml
---
apiVersion: vpn.ahova.com/v1alpha1
kind: Wireguard
metadata:
  name: wireguard-sidecar
spec:
  address: 192.168.2.1/24
  sidecars:
    - name: exporter
      image: docker.io/mindflavor/prometheus-wireguard-exporter:3.6.6
      args:
        - --export_latest_handshake_delay
        - "true"
        - --verbose
        - "true"
        - --extract_names_config_files
        - /config/wg0.conf
      ports:
        - containerPort: 9586
          name: exporter
          protocol: TCP
      volumeMounts:
        - name: config
          readOnly: true
          mountPath: /config
      securityContext:
        runAsUser: 0
        runAsGroup: 0
        capabilities:
          add:
            - NET_ADMIN

---
apiVersion: vpn.ahova.com/v1alpha1
kind: WireguardPeer
metadata:
  name: peer-sidecar
spec:
  wireguardRef: wireguard-sidecar
  address: 192.168.2.2/32
```

Highly available setup
```yaml
---
apiVersion: vpn.ahova.com/v1alpha1
kind: Wireguard
metadata:
  name: wireguard-ha
spec:
  address: 192.168.3.1/24
  replicas: 3
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - topologyKey: topology.kubernetes.io/zone
          labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/instance
                operator: In
                values:
                  - wireguard-ha

---
apiVersion: vpn.ahova.com/v1alpha1
kind: WireguardPeer
metadata:
  name: peer-ha
spec:
  wireguardRef: wireguard-ha
  address: 192.168.3.2/32
```
