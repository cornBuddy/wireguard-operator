package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var _ = Describe("WireguardPeer#Secret", func() {
	It("Endpoint should equal `clusterIP:wireguard-port` by default", func() {
		By("provisioning corresponding wireguard CRD")
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{})
		Eventually(func() error {
			return wgDsl.Apply(ctx, wg)
		}, timeout, interval).Should(Succeed())

		By("provisioning peer CRD")
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
		})
		Eventually(func() error {
			return peerDsl.Apply(ctx, peer)
		}, timeout, interval).Should(Succeed())

		By("fetching secret")
		secret := &v1.Secret{}
		key := types.NamespacedName{
			Name:      peer.GetName(),
			Namespace: peer.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, key, secret)).To(Succeed())

		By("fetching wireguard service")
		wgSvc := &v1.Service{}
		wgKey := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, wgKey, wgSvc)).To(Succeed())

		By("asserting Endpoint section of .data.config")
		config := string(secret.Data["config"])
		ep := fmt.Sprintf("Endpoint = %s:%d\n", wgSvc.Spec.ClusterIP, wireguardPort)
		Expect(config).To(ContainSubstring(ep))
	})

	It("Endpoint should equal `externalIP:wireguard-port` when wireguardRef.spec.serviceType == LoadBalancer", func() {
		By("provisioning corresponding wireguard CRD")
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
			ServiceType: v1.ServiceTypeLoadBalancer,
		})
		Eventually(func() error {
			return wgDsl.Apply(ctx, wg)
		}, timeout, interval).Should(Succeed())

		By("fetching wireguard service")
		wgSvc := &v1.Service{}
		wgKey := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, wgKey, wgSvc)).To(Succeed())

		By("mocking public ip")
		wgSvc.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{{
			IP: "127.0.0.1",
		}}
		Expect(wgDsl.Reconciler.Status().Update(ctx, wgSvc)).To(Succeed())

		By("provisioning peer CRD")
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
		})
		Eventually(func() error {
			return peerDsl.Apply(ctx, peer)
		}, timeout, interval).Should(Succeed())

		By("fetching secret")
		secret := &v1.Secret{}
		key := types.NamespacedName{
			Name:      peer.GetName(),
			Namespace: peer.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, key, secret)).To(Succeed())

		By("asserting ndpoint section of .data.config")
		config := string(secret.Data["config"])
		ep := wgSvc.Status.LoadBalancer.Ingress[0].IP
		endpoint := fmt.Sprintf("Endpoint = %s:%d\n", ep, wireguardPort)
		Expect(config).To(ContainSubstring(endpoint))
	})

	It("Endpoint should equal `endpointAddress:wireguard-port` when wireguardRef.spec.endpointAddress != nil", func() {
		By("provisioning corresponding wireguard CRD")
		addr := "localhost"
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
			EndpointAddress: &addr,
		})
		Eventually(func() error {
			return wgDsl.Apply(ctx, wg)
		}, timeout, interval).Should(Succeed())

		By("provisioning peer CRD")
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
		})
		Eventually(func() error {
			return peerDsl.Apply(ctx, peer)
		}, timeout, interval).Should(Succeed())

		By("fetching secret")
		secret := &v1.Secret{}
		key := types.NamespacedName{
			Name:      peer.GetName(),
			Namespace: peer.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, key, secret)).To(Succeed())

		By("asserting Endpoint section of .data.config")
		config := string(secret.Data["config"])
		ep := fmt.Sprintf("Endpoint = %s:%d\n", *wg.Spec.EndpointAddress, wireguardPort)
		Expect(config).To(ContainSubstring(ep))
	})
})
