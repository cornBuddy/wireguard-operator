# wireguard-operator

Kubernetes operator to provision wireguard peers

[Documentation](https://cornbuddy.github.io/wireguard-operator/)

## prerequisites

The following
[unsafe sysctls](https://kubernetes.io/docs/tasks/administer-cluster/sysctl-cluster/#safe-and-unsafe-sysctls)
must be allowed:
* `net.ipv4.ip_forward`
* `net.ipv4.conf.all.src_valid_mark`
* `net.ipv4.conf.all.rp_filter`
* `net.ipv4.conf.all.route_localnet`

## tl;dr

```bash
kubectl apply -f \
    https://github.com/cornbuddy/wireguard-operator/raw/refs/heads/main/src/config/manifest.yml

echo "
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
  wireguardRef: vpn"

kubectl get secret -o json peer \
    | jq -r '.data.config' \
    | base64 -d > /etc/wireguard/wg0.conf
sudo wg-quick up wg0
```
