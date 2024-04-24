package factory

import (
	"fmt"
	"testing"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/test/dsl"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/stretchr/testify/assert"
)

func TestPeerDefaultConfigurations(t *testing.T) {
	t.Parallel()

	key, err := wgtypes.GeneratePrivateKey()
	assert.Nil(t, err)

	ep := "127.0.0.1:51820"
	wantPrivKey := key.String()
	wantPubKey := key.PublicKey().String()
	secret, err := defaultPeerFact.Secret(ep, wantPubKey, wantPrivKey)
	assert.Nil(t, err)

	assert.Contains(t, secret.Data, "private-key")
	assert.Contains(t, secret.Data, "public-key")
	assert.Equal(t, string(secret.Data["private-key"]), wantPrivKey)
	assert.Equal(t, string(secret.Data["public-key"]), wantPubKey)

	gotPubKey := string(secret.Data["public-key"])
	assert.Equal(t, wantPubKey, gotPubKey)

	config := string(secret.Data["config"])
	assert.NotEmpty(t, config)

	peerSpec := defaultPeerFact.Peer.Spec
	wgStatus := defaultPeerFact.Wireguard.Status
	configLines := []string{
		"PersistentKeepalive = 25",
		"DNS = 127.0.0.1",
		"AllowedIPs = 0.0.0.0/0",
		fmt.Sprintf("Endpoint = %s", ep),
		fmt.Sprintf("PrivateKey = %s", wantPrivKey),
		fmt.Sprintf("Address = %s", peerSpec.Address),
		fmt.Sprintf("PublicKey = %s", *wgStatus.PublicKey),
	}
	for _, line := range configLines {
		assert.Contains(t, config, line)
	}
}

func TestPeerKeyIsProvided(t *testing.T) {
	t.Parallel()

	key, err := wgtypes.GeneratePrivateKey()
	assert.Nil(t, err)

	publicKey := key.PublicKey().String()
	peer := dsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
		PublicKey: &publicKey,
	}, v1alpha1.WireguardPeerStatus{PublicKey: &publicKey})
	wireguard := dsl.GenerateWireguard(
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
	secret, err := fact.Secret("kekeke", "kekeke", "kekeke")
	assert.Nil(t, err)
	assert.NotEmpty(t, secret, "should produce secret")

	assert.Equal(t, peer.GetName(), secret.GetName())
	assert.Equal(t, peer.GetNamespace(), secret.GetNamespace())

	assert.Contains(t, secret.Data, "public-key")
	assert.NotEmpty(t, secret.Data["public-key"])
	assert.Equal(t, publicKey, string(secret.Data["public-key"]))

	assert.NotContains(t, secret.Data, "config")
	assert.NotContains(t, secret.Data, "private-key")
}

func TestPeerDecorations(t *testing.T) {
	t.Parallel()

	secret, err := defaultPeerFact.Secret("kekeke", "kekeke", "kekeke")
	assert.Nil(t, err)

	shouldHaveProperDecorations(t, secret)
}
