package dsl

import (
	"fmt"

	"k8s.io/api/core/v1"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

var (
	ErrPublicIpNotSet = fmt.Errorf("public ip is not set")
)

type Peer struct {
	PeerSpec      vpnv1alpha1.WireguardPeerSpec
	WireguardSpec vpnv1alpha1.WireguardSpec
}

func (p Peer) ExtractEndpoint(svc v1.Service) (string, error) {
	publicIpIsNotSet := p.WireguardSpec.ServiceType == v1.ServiceTypeLoadBalancer &&
		len(svc.Status.LoadBalancer.Ingress) == 0
	if publicIpIsNotSet {
		return "", ErrPublicIpNotSet
	}

	endpoint := ""
	if p.WireguardSpec.EndpointAddress != nil {
		endpoint = *p.WireguardSpec.EndpointAddress
	} else if p.WireguardSpec.ServiceType == v1.ServiceTypeClusterIP {
		endpoint = svc.Spec.ClusterIP
	} else if p.WireguardSpec.ServiceType == v1.ServiceTypeLoadBalancer {
		endpoint = svc.Status.LoadBalancer.Ingress[0].IP
	}

	if endpoint == "" {
		return "", fmt.Errorf("cannot retrieve endpoint")
	}

	return endpoint, nil
}
