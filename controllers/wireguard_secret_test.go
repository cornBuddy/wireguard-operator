package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var _ = Describe("Wireguard#Secret", func() {
	var wg *vpnv1alpha1.Wireguard
	secret := &v1.Secret{}
	postUps := []TableEntry{
		Entry(
			nil,
			"PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 192.168.0.0/16 --jump DROP",
		),
		Entry(
			nil,
			"PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 172.16.0.0/12 --jump DROP",
		),
		Entry(
			nil,
			"PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 10.0.0.0/8 --jump DROP",
		),
		Entry(
			nil,
			"PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 169.254.169.254/32 --jump DROP",
		),
		Entry(
			nil,
			"PostUp = iptables --append FORWARD --in-interface %i --jump ACCEPT",
		),
		Entry(
			nil,
			"PostUp = iptables --append FORWARD --out-interface %i --jump ACCEPT",
		),
		Entry(
			nil,
			"PostUp = iptables --table nat --append POSTROUTING --source 192.168.254.1/24 --out-interface eth0 --jump MASQUERADE",
		),
	}
	peers := []TableEntry{
		Entry(
			"when peer is default",
			testdsl.GeneratePeer(vpnv1alpha1.WireguardPeerSpec{}),
		),
	}

	BeforeEach(func() {
		By("creating wireguard CRD")
		wg = testdsl.GenerateWireguard(vpnv1alpha1.WireguardSpec{})
		Eventually(func() error {
			return k8sClient.Create(ctx, wg)
		}, timeout, interval).Should(Succeed())
		Expect(wgDsl.Reconcile(wg)).To(Succeed())

		By("retrieving secret from cluster")
		key := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, key, secret)).To(Succeed())
	})

	AfterEach(func() {
		By("deleting wireguard CRD")
		Eventually(func() error {
			return k8sClient.Delete(ctx, wg)
		}, timeout, interval).Should(Succeed())
	})

	It("should be valid", func() {
		By("checking that required keys are set")
		Expect(secret.Data).To(HaveKey("config"))
		Expect(secret.Data).To(HaveKey("public-key"))
		Expect(secret.Data).To(HaveKey("private-key"))

		By("validating keys lenght")
		const keyLength = 44
		Expect(secret.Data["public-key"]).To(HaveLen(keyLength))
		Expect(secret.Data["private-key"]).To(HaveLen(keyLength))

		By("checking private keys is parsable")
		privateKey := string(secret.Data["private-key"])
		wgKey, err := wgtypes.ParseKey(privateKey)
		Expect(err).To(BeNil())

		By("checking public key corresponds to the private key")
		pubKey := wgKey.PublicKey().String()
		Expect(string(secret.Data["public-key"])).To(Equal(pubKey))

		By("checking that 'last-applied' annotation set")
		annotations := secret.GetAnnotations()
		Expect(annotations).To(HaveKey("vpn.ahova.com/last-applied"))
	})

	It("should have valid [Interface] section", func() {
		config := string(secret.Data["config"])
		address := fmt.Sprintf("Address = %s", wg.Spec.Network)
		Expect(config).To(ContainSubstring(address))

		privKey := fmt.Sprintf("PrivateKey = %s", secret.Data["private-key"])
		Expect(config).To(ContainSubstring(privKey))

		Expect(config).To(ContainSubstring("ListenPort = 51820"))
	})

	DescribeTable("should have required PostUps", func(postUp string) {
		config := string(secret.Data["config"])
		Expect(config).To(ContainSubstring(postUp))
	}, postUps)

	DescribeTable("has valid [Peer] section", func(peer *vpnv1alpha1.WireguardPeer) {
		By("creating Peer CRD")
		Eventually(func() error {
			peer.Spec.WireguardRef = wg.GetName()
			return k8sClient.Create(ctx, peer)
		}, timeout, interval).Should(Succeed())
		Expect(peerDsl.Reconcile(peer)).To(Succeed())

		By("reconciling Wireguard CRD")
		Expect(wgDsl.Reconcile(wg)).To(Succeed())

		By("refetching Wireguard secret from cluster")
		wgKey := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, wgKey, secret)).To(Succeed())

		By("fetching peer secret")
		peerKey := types.NamespacedName{
			Name:      peer.GetName(),
			Namespace: peer.GetNamespace(),
		}
		peerSecret := &v1.Secret{}
		Expect(k8sClient.Get(ctx, peerKey, peerSecret)).To(Succeed())

		By("fetching wireguard service")
		svc := &v1.Service{}
		Expect(k8sClient.Get(ctx, wgKey, svc)).To(Succeed())

		By("checking that [Peer] section exists")
		config := string(secret.Data["config"])
		Expect(config).To(ContainSubstring("[Peer]"))

		By("validating PublicKey configuration")
		pubKey := fmt.Sprintf("PublicKey = %s", peerSecret.Data["public-key"])
		Expect(config).To(ContainSubstring(pubKey))

		By("validating AllowedIPs configuration")
		ip := fmt.Sprintf("AllowedIPs = %s/32", peer.Spec.Address)
		Expect(config).To(ContainSubstring(ip))

		By("validating Endpoint configuration")
		ep := fmt.Sprintf("Endpoint = %s:51820", svc.Spec.ClusterIP)
		Expect(config).To(ContainSubstring(ep))

		By("validating PersistentKeepalive configuration")
		Expect(config).To(ContainSubstring("PersistentKeepalive = 25"))
	}, peers)
})
