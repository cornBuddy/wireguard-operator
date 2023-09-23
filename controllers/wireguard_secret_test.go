package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var _ = Describe("Wireguard#Secret defaults", func() {
	var wg v1alpha1.Wireguard
	var wgKey types.NamespacedName
	secret := &v1.Secret{}

	BeforeEach(func() {
		By("creating wireguard CRD")
		wg = testdsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		Eventually(func() error {
			return k8sClient.Create(ctx, &wg)
		}, timeout, interval).Should(Succeed())
		Expect(wgDsl.Reconcile(&wg)).To(Succeed())

		By("retrieving secret from cluster")
		wgKey = types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, wgKey, secret)).To(Succeed())
	})

	AfterEach(func() {
		By("deleting wireguard CRD")
		Eventually(func() error {
			return k8sClient.Delete(ctx, &wg)
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

	It("should not regenerate creds", func() {
		By("simulating reconcilation loop cycle for wireguard CRD")
		Expect(wgDsl.Reconcile(&wg)).To(Succeed())

		By("refetching the secret")
		gotSecret := &v1.Secret{}
		Expect(k8sClient.Get(ctx, wgKey, gotSecret)).To(Succeed())

		By("ensuring that were not deleted")
		Expect(gotSecret.Data).To(HaveKey("public-key"))
		Expect(gotSecret.Data).To(HaveKey("private-key"))

		By("validating public key")
		gotPubKey := gotSecret.Data["public-key"]
		wantPubKey := secret.Data["public-key"]
		Expect(wantPubKey).To(Equal(gotPubKey))

		By("validating private key")
		gotPrivKey := gotSecret.Data["private-key"]
		wantPrivKey := secret.Data["private-key"]
		Expect(gotPrivKey).To(Equal(wantPrivKey))
	})

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
			"PostUp = iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE",
		),
	}
	DescribeTable("should have required PostUps", func(postUp string) {
		config := string(secret.Data["config"])
		Expect(config).To(ContainSubstring(postUp))
	}, postUps)

	peers := []TableEntry{
		Entry(
			"when peer is default",
			testdsl.GeneratePeer(
				v1alpha1.WireguardPeerSpec{},
				v1alpha1.WireguardPeerStatus{},
			),
		),
	}
	DescribeTable("has valid [Peer] section", func(peer v1alpha1.WireguardPeer) {
		By("creating Peer CRD")
		Eventually(func() error {
			peer.Spec.WireguardRef = wg.GetName()
			return k8sClient.Create(ctx, &peer)
		}, timeout, interval).Should(Succeed())
		Expect(peerDsl.Reconcile(&peer)).To(Succeed())

		By("reconciling Wireguard CRD")
		Expect(wgDsl.Reconcile(&wg)).To(Succeed())

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
		Expect(config).To(Not(ContainSubstring("Endpoint =")))

		By("validating PersistentKeepalive configuration")
		Expect(config).To(ContainSubstring("PersistentKeepalive = 25"))

		By("validating SaveConfig configuration")
		Expect(config).To(ContainSubstring("SaveConfig = false"))
	}, peers)
})
