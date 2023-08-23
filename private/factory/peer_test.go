package factory

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var _ = Describe("Peer#Secret", func() {
	It("should have proper decorations", func() {
		secret, err := peerFactory.Secret("kekeke")
		Expect(err).To(BeNil())
		shouldHaveProperDecorations(secret)
	})

	It("should autogenerate keys by default", func() {
		secret, err := peerFactory.Secret("kekeke")
		Expect(err).To(BeNil())
		Expect(secret).ToNot(BeNil())

		Expect(secret.GetName()).To(Equal(defaultPeer.GetName()))
		Expect(secret.GetNamespace()).To(Equal(defaultPeer.GetNamespace()))

		Expect(secret.Data).To(HaveKey("config"))
		Expect(secret.Data).To(HaveKey("public-key"))
		Expect(secret.Data).To(HaveKey("private-key"))

		Expect(secret.Data["config"]).ToNot(BeEmpty())
		Expect(secret.Data["public-key"]).ToNot(BeEmpty())
		Expect(secret.Data["private-key"]).ToNot(BeEmpty())

		key, err := wgtypes.ParseKey(string(secret.Data["private-key"]))
		Expect(err).To(BeNil())
		wantPubKey := string(secret.Data["public-key"])
		Expect(key.PublicKey().String()).To(Equal(wantPubKey))
	})

	It("should not autogenerate keys when peer.spec.publicKey is set", func() {
		key, err := wgtypes.GeneratePrivateKey()
		Expect(err).To(BeNil())

		publicKey := key.PublicKey().String()
		peer := testdsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			PublicKey: &publicKey,
		}, v1alpha1.WireguardPeerStatus{PublicKey: &publicKey})
		wireguard := testdsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{
				Endpoint:  toPtr("kekeke"),
				PublicKey: toPtr("kekeke"),
			},
		)
		fact := Peer{
			Scheme:    scheme,
			Peer:      peer,
			Wireguard: wireguard,
		}
		secret, err := fact.Secret("kekeke")
		Expect(err).To(BeNil())
		Expect(secret).ToNot(BeNil())

		Expect(secret.GetName()).To(Equal(peer.GetName()))
		Expect(secret.GetNamespace()).To(Equal(peer.GetNamespace()))

		Expect(secret.Data).To(HaveKey("public-key"))
		Expect(secret.Data["public-key"]).ToNot(BeEmpty())
		Expect(string(secret.Data["public-key"])).To(Equal(publicKey))

		Expect(secret.Data).ToNot(HaveKey("config"))
		Expect(secret.Data).ToNot(HaveKey("private-key"))
	})
})
