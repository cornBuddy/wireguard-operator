---
title: API Reference
nav_order: 1
---

# API Reference
{: .no_toc }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

## Packages
- [vpn.ahova.com/v1alpha1](#vpnahovacomv1alpha1)


## vpn.ahova.com/v1alpha1


### Resource Types
- [Wireguard](#wireguard)
- [WireguardPeer](#wireguardpeer)



#### Address

_Underlying type:_ _string_

IP address of the peer

_Validation:_
- Pattern: `^((10(\.(([0-9]?[0-9])|(1[0-9]?[0-9])|(2[0-4]?[0-9])|(25[0-5]))){3})|(172\.((1[6-9])|(2[0-9])(3[0-1]))(\.(([0-9]?[0-9])|(1[0-9]?[0-9])|(2[0-4]?[0-9])|(25[0-5]))){2})|(192\.168(\.(([0-9]?[0-9])|(1[0-9]?[0-9])|(2[0-4]?[0-9])|(25[0-5]))){2})|(127('\.(([0-9]?[0-9])|(1[0-9]?[0-9])|(2[0-4]?[0-9])|(25[0-5]))){3}))/([8-9]|(1[0-9])|(2[0-9])|(3[0-2]))$`

_Appears in:_
- [WireguardPeerSpec](#wireguardpeerspec)
- [WireguardSpec](#wireguardspec)



#### Wireguard



WireguardPeer is the Schema for the wireguardpeers API





| Field | Description | Default |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpn.ahova.com/v1alpha1` | |
| `kind` _string_ | `Wireguard` | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata` |  |
| `spec` _[WireguardSpec](#wireguardspec)_ |  |  |
| `status` _[WireguardStatus](#wireguardstatus)_ |  |  |


#### WireguardPeer



WireguardPeer is the Schema for the wireguardpeers API





| Field | Description | Default |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpn.ahova.com/v1alpha1` | |
| `kind` _string_ | `WireguardPeer` | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata` |  |
| `spec` _[WireguardPeerSpec](#wireguardpeerspec)_ |  |  |
| `status` _[WireguardPeerStatus](#wireguardpeerstatus)_ |  |  |


#### WireguardPeerSpec



WireguardPeerSpec defines the desired state of Wireguard



_Appears in:_
- [WireguardPeer](#wireguardpeer)

| Field | Description | Default |
| --- | --- | --- | --- |
| `address` _[Address](#address)_ | IP address of the peer | 192.168.254.2/24 |
| `wireguardRef` _string_ | Required. Reference to the wireguard resource |  |
| `publicKey` _string_ | Public key of the peer |  |


#### WireguardPeerStatus







_Appears in:_
- [WireguardPeer](#wireguardpeer)

| Field | Description | Default |
| --- | --- | --- | --- |
| `publicKey` _string_ | Public key of the peer |  |


#### WireguardSpec







_Appears in:_
- [Wireguard](#wireguard)

| Field | Description | Default |
| --- | --- | --- | --- |
| `replicas` _integer_ | Replicas defines the number of Wireguard instances | 1 |
| `serviceType` _[ServiceType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#servicetype-v1-core)_ | Type of the service to be created | ClusterIP |
| `allowedIPs` _string_ | IP addresses allowed to be routed | 0.0.0.0/0 |
| `address` _[Address](#address)_ | Address space to use | 192.168.254.1/24 |
| `dns` _string_ | DNS configuration for peer | 1.1.1.1 |
| `endpointAddress` _string_ | Address which going to be used in peers configuration. By default,<br />operator will use IP address of the service, which is not always<br />desirable (e.g. if public DNS record is attached to load balancer) |  |
| `dropConnectionsTo` _string array_ | Deny connections to the following list of IPs |  |
| `sidecars` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#container-v1-core) array_ | Sidecar containers to run |  |
| `affinity` _[Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#affinity-v1-core)_ | Affinity configuration |  |
| `serviceAnnotations` _object (keys:string, values:string)_ | Annotations for the service resource |  |
| `labels` _object (keys:string, values:string)_ | Extra labels for all resources created |  |


#### WireguardStatus







_Appears in:_
- [Wireguard](#wireguard)

| Field | Description | Default |
| --- | --- | --- | --- |
| `publicKey` _string_ | Public key of the peer |  |
| `endpoint` _string_ | Endpoint of the peer |  |


