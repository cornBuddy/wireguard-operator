package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

var _ = Describe("WireguardPeer controller", func() {
	const wireguardName = "wireguard"
	var wireguard *vpnv1alpha1.Wireguard

	BeforeEach(func() {
		By("Checking if wireguard CR reconciled successfully")
		wireguard = &vpnv1alpha1.Wireguard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wireguardName,
				Namespace: corev1.NamespaceDefault,
			},
			Spec: vpnv1alpha1.WireguardSpec{},
		}
		validateReconcile(wireguard, wgDsl)
	})

	AfterEach(func() {
		By("Deleting wireguard CRs")
		wireguard = nil
		wg := &vpnv1alpha1.Wireguard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wireguardName,
				Namespace: corev1.NamespaceDefault,
			},
		}
		Expect(k8sClient.Delete(context.TODO(), wg)).To(Succeed())

		By("Deleting wireguard peer CRs")
		peer := &vpnv1alpha1.WireguardPeer{}
		err := k8sClient.DeleteAllOf(context.TODO(), peer)
		deletedOrNotFound := err == nil || apierrors.IsNotFound(err)
		Expect(deletedOrNotFound).To(BeTrue())
	})

	DescribeTable("should reconcile",
		func(peer *vpnv1alpha1.WireguardPeer) {
			validateReconcile(peer, peerDsl)
			validatePeerSecret(peer, wireguard)
		},
		Entry(
			"default configuration",
			&vpnv1alpha1.WireguardPeer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "peer-default",
					Namespace: corev1.NamespaceDefault,
				},
				Spec: vpnv1alpha1.WireguardPeerSpec{
					WireguardRef: wireguardName,
				},
			},
		),
		Entry(
			"public key configuration",
			&vpnv1alpha1.WireguardPeer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "peer-public-key",
					Namespace: corev1.NamespaceDefault,
				},
				Spec: vpnv1alpha1.WireguardPeerSpec{
					WireguardRef: wireguardName,
					PublicKey:    toPtr("WsFemZZdyC+ajbvOtKA7dltaNCaPOusKmkJffjMOMmg="),
				},
			},
		),
	)
})

func validatePeerSecret(peer *vpnv1alpha1.WireguardPeer, wireguard *vpnv1alpha1.Wireguard) {
	key := types.NamespacedName{
		Name:      peer.GetName(),
		Namespace: peer.GetNamespace(),
	}
	secret := &corev1.Secret{}
	const keyLength = 44

	By("Checking if Secret were created successfully during reconcilation")
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(context.TODO(), key, secret)).To(Succeed())
		g.Expect(secret.Data).To(HaveKey("public-key"))
		g.Expect(secret.Data["public-key"]).To(HaveLen(keyLength))

		if peer.Spec.PublicKey == nil {
			g.Expect(secret.Data).To(HaveKey("config"))
			g.Expect(secret.Data).To(HaveKey("private-key"))

			g.Expect(secret.Data["private-key"]).To(HaveLen(keyLength))
			privateKey := string(secret.Data["private-key"])
			peerKey, err := wgtypes.ParseKey(privateKey)
			g.Expect(err).To(BeNil())
			pubKey := peerKey.PublicKey().String()
			g.Expect(string(secret.Data["public-key"])).To(Equal(pubKey))

			cfg := string(secret.Data["config"])
			addr := fmt.Sprintf("Address = %s/32", peer.Spec.Address)
			g.Expect(cfg).To(ContainSubstring(addr))

			privKeyConfig := fmt.Sprintf("PrivateKey = %s", privateKey)
			g.Expect(cfg).To(ContainSubstring(privKeyConfig))

			dns := fmt.Sprintf("DNS = %s", wireguard.Spec.DNS.Address)
			g.Expect(cfg).To(ContainSubstring(dns))

			wgKey := types.NamespacedName{
				Name:      wireguard.GetName(),
				Namespace: wireguard.GetNamespace(),
			}
			wgSecret := &corev1.Secret{}
			g.Expect(k8sClient.Get(context.TODO(), wgKey, wgSecret)).To(Succeed())
			pubKeyConfig := fmt.Sprintf("PublicKey = %s", wgSecret.Data["public-key"])
			g.Expect(cfg).To(ContainSubstring(pubKeyConfig))

			ep := fmt.Sprintf("Endpoint = %s", wireguard.Spec.EndpointAddress)
			g.Expect(cfg).To(ContainSubstring(ep))

			ips := "AllowedIPs = 0.0.0.0/0, ::/0"
			g.Expect(cfg).To(ContainSubstring(ips))
		} else {
			g.Expect(secret.Data).ToNot(HaveKey("config"))
			g.Expect(secret.Data).ToNot(HaveKey("private-key"))
		}
	}, timeout, interval).Should(Succeed())
}
