package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var (
	defaultWireguard = v1alpha1.Wireguard{}
	defaultPeer      = v1alpha1.WireguardPeer{}
	secret           = &v1.Secret{}
)

var _ = Describe("WireguardPeer#Secret", func() {
	BeforeEach(func() {
		By("generating test CRDs")
		defaultWireguard = testdsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		defaultPeer = testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: defaultWireguard.GetName(),
		}, v1alpha1.WireguardPeerStatus{})

		By("provisioning corresponding wireguard CRD")
		Eventually(func() error {
			return wgDsl.Apply(ctx, &defaultWireguard)
		}, timeout, interval).Should(Succeed())

		By("provisioning peer CRD")
		Eventually(func() error {
			return peerDsl.Apply(ctx, &defaultPeer)
		}, timeout, interval).Should(Succeed())

		By("fetching secret")
		key := types.NamespacedName{
			Name:      defaultPeer.GetName(),
			Namespace: defaultPeer.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, key, secret)).To(Succeed())
	})

	AfterEach(func() {
		defaultWireguard = v1alpha1.Wireguard{}
		defaultPeer = v1alpha1.WireguardPeer{}
		secret = &v1.Secret{}
	})

	It("works by default", func() {
		By("validating secret")
		Expect(secret.Data).To(HaveKey("config"))
		Expect(secret.Data).To(HaveKey("private-key"))
		Expect(secret.Data).To(HaveKey("public-key"))

		Expect(secret.Data["private-key"]).To(HaveLen(keyLength))
		Expect(secret.Data["public-key"]).To(HaveLen(keyLength))

		key := string(secret.Data["private-key"])
		peerCreds, err := wgtypes.ParseKey(key)
		Expect(err).To(BeNil())

		pubKey := peerCreds.PublicKey().String()
		Expect(string(secret.Data["public-key"])).To(Equal(pubKey))

		config := string(secret.Data["config"])
		addr := fmt.Sprintf("Address = %s/32", defaultPeer.Spec.Address)
		Expect(config).To(ContainSubstring(addr))

		privKeyConfig := fmt.Sprintf("PrivateKey = %s", key)
		Expect(config).To(ContainSubstring(privKeyConfig))

		Expect(config).To(ContainSubstring("DNS = 127.0.0.1"))

		wgKey := types.NamespacedName{
			Name:      defaultWireguard.GetName(),
			Namespace: defaultWireguard.GetNamespace(),
		}
		wgSecret := &v1.Secret{}
		Expect(k8sClient.Get(ctx, wgKey, wgSecret)).To(Succeed())
		pubKeyConfig := fmt.Sprintf("PublicKey = %s", wgSecret.Data["public-key"])
		Expect(config).To(ContainSubstring(pubKeyConfig))

		ips := "AllowedIPs = 0.0.0.0/0\n"
		Expect(config).To(ContainSubstring(ips))
	})

	It("Endpoint should equal `clusterIP:wireguard-port` by default", func() {
		By("fetching wireguard service")
		wgSvc := &v1.Service{}
		wgKey := types.NamespacedName{
			Name:      defaultWireguard.GetName(),
			Namespace: defaultWireguard.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, wgKey, wgSvc)).To(Succeed())

		By("asserting Endpoint section of .data.config")
		config := string(secret.Data["config"])
		ep := fmt.Sprintf("Endpoint = %s:%d\n", wgSvc.Spec.ClusterIP, wireguardPort)
		Expect(config).To(ContainSubstring(ep))
	})

	It("Endpoint should equal `clusterIp:wireguard-port` when wireguardRef.spec.serviceType == LoadBalancer and svc.status is not set", func() {
		By("provisioning corresponding wireguard CRD")
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
			ServiceType: v1.ServiceTypeLoadBalancer,
		}, v1alpha1.WireguardStatus{})
		Eventually(func() error {
			return wgDsl.Apply(ctx, &wg)
		}, timeout, interval).Should(Succeed())

		By("fetching wireguard service")
		wgSvc := &v1.Service{}
		wgKey := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, wgKey, wgSvc)).To(Succeed())

		By("provisioning peer CRD")
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
		}, v1alpha1.WireguardPeerStatus{})
		Eventually(func() error {
			return peerDsl.Apply(ctx, &peer)
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
		ep := wgSvc.Spec.ClusterIP
		endpoint := fmt.Sprintf("Endpoint = %s:%d\n", ep, wireguardPort)
		Expect(config).To(ContainSubstring(endpoint))
	})

	It("Endpoint should equal `endpointAddress:wireguard-port` when wireguardRef.spec.endpointAddress != nil", func() {
		By("provisioning corresponding wireguard CRD")
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
			EndpointAddress: toPtr("localhost"),
		}, v1alpha1.WireguardStatus{})
		Eventually(func() error {
			return wgDsl.Apply(ctx, &wg)
		}, timeout, interval).Should(Succeed())

		By("provisioning peer CRD")
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
		}, v1alpha1.WireguardPeerStatus{})
		Eventually(func() error {
			return peerDsl.Apply(ctx, &peer)
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

	It("should have only public key set when peer.spec.publicKey is set", func() {
		By("generating key")
		creds, err := wgtypes.GeneratePrivateKey()
		Expect(err).To(BeNil())

		By("provisioning peer CRD")
		publicKey := toPtr(creds.PublicKey().String())
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: defaultWireguard.GetName(),
			PublicKey:    publicKey,
		}, v1alpha1.WireguardPeerStatus{})
		Eventually(func() error {
			return peerDsl.Apply(ctx, &peer)
		}, timeout, interval).Should(Succeed())

		By("fetching secret")
		secret := &v1.Secret{}
		key := types.NamespacedName{
			Name:      peer.GetName(),
			Namespace: peer.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, key, secret)).To(Succeed())

		By("validating secret")
		data := secret.Data
		Expect(data).ToNot(HaveKey("config"))
		Expect(data).ToNot(HaveKey("private-key"))
		Expect(data).To(HaveKey("public-key"))
		Expect(data["public-key"]).To(HaveLen(keyLength))
	})
})
