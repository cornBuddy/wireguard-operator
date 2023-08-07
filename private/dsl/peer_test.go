package dsl

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/api/core/v1"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

func TestPeer(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Peer")
}

var _ = Describe("ExtractEndpoint", func() {
	It("should return cluster ip when wg.ServiceType == ClusterIP", func() {
		peerSpec := vpnv1alpha1.WireguardPeerSpec{}
		wireguardSpec := vpnv1alpha1.WireguardSpec{
			ServiceType: v1.ServiceTypeClusterIP,
		}
		peer := Peer{
			PeerSpec:      peerSpec,
			WireguardSpec: wireguardSpec,
		}
		wireguardService := v1.Service{
			Spec: v1.ServiceSpec{
				ClusterIP: "127.0.0.1",
			},
		}
		ep, err := peer.ExtractEndpoint(wireguardService)

		Expect(err).To(BeNil())
		Expect(ep).To(Equal(wireguardService.Spec.ClusterIP))
	})

	It("should return public ip when wg.ServiceType == LoadBalancer", func() {
		peerSpec := vpnv1alpha1.WireguardPeerSpec{}
		wireguardSpec := vpnv1alpha1.WireguardSpec{
			ServiceType: v1.ServiceTypeLoadBalancer,
		}
		peer := Peer{
			PeerSpec:      peerSpec,
			WireguardSpec: wireguardSpec,
		}
		wireguardService := v1.Service{
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{{
						IP: "127.0.0.1",
					}},
				},
			},
		}
		ep, err := peer.ExtractEndpoint(wireguardService)
		publicIp := wireguardService.Status.LoadBalancer.Ingress[0].IP

		Expect(err).To(BeNil())
		Expect(ep).To(Equal(publicIp))
	})

	It("should return error when wg.ServiceType == LoadBalancer, but no public ip set", func() {
		peerSpec := vpnv1alpha1.WireguardPeerSpec{}
		wireguardSpec := vpnv1alpha1.WireguardSpec{
			ServiceType: v1.ServiceTypeLoadBalancer,
		}
		peer := Peer{
			PeerSpec:      peerSpec,
			WireguardSpec: wireguardSpec,
		}
		wireguardService := v1.Service{}
		ep, err := peer.ExtractEndpoint(wireguardService)

		Expect(err).To(Equal(ErrPublicIpNotSet))
		Expect(ep).To(BeEmpty())
	})
})
