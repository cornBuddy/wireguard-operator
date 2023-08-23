package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var _ = Describe("WireguardPeer.Status", func() {
	It("should set public key to status when explicitly defined in the spec", func() {
		By("generating keypair")
		creds, err := wgtypes.GeneratePrivateKey()
		Expect(err).To(BeNil())

		By("applying CRDs")
		wg := testdsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		pubKey := creds.PublicKey().String()
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
			PublicKey:    toPtr(pubKey),
		}, v1alpha1.WireguardPeerStatus{})
		Eventually(func() error {
			return wgDsl.Apply(ctx, &wg)
		}, timeout, interval).Should(Succeed())
		Eventually(func() error {
			return peerDsl.Apply(ctx, &peer)
		}, timeout, interval).Should(Succeed())

		By("refetching peer CRD")
		key := types.NamespacedName{
			Namespace: peer.GetNamespace(),
			Name:      peer.GetName(),
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, &peer)
		}, timeout, interval).Should(Succeed())

		By("validating status")
		Expect(*peer.Status.PublicKey).To(Equal(pubKey))
	})

	It("should set public key to status when after it's written in the secret", func() {
		By("applying CRDs")
		wg := testdsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
		}, v1alpha1.WireguardPeerStatus{})
		Eventually(func() error {
			return wgDsl.Apply(ctx, &wg)
		}, timeout, interval).Should(Succeed())
		Eventually(func() error {
			return peerDsl.Apply(ctx, &peer)
		}, timeout, interval).Should(Succeed())

		By("refetching peer")
		key := types.NamespacedName{
			Namespace: peer.GetNamespace(),
			Name:      peer.GetName(),
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, &peer)
		}, timeout, interval).Should(Succeed())

		By("fetching secret")
		secret := &v1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, secret)
		}, timeout, interval).Should(Succeed())

		By("validating status")
		pubKey := string(secret.Data["public-key"])
		Expect(*peer.Status.PublicKey).To(Equal(pubKey))
	})
})
