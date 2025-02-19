package factory

import (
	"fmt"
	"testing"

	"github.com/poy/onpar"
	"github.com/stretchr/testify/assert"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/ahova/ahova-vpn/services/wireguard-operator/api/v1alpha1"
	"github.com/ahova/ahova-vpn/services/wireguard-operator/test/dsl"
)

func TestPeerDnsConfigurations(t *testing.T) {
	t.Parallel()

	type testContext struct {
		privKey  string
		pubKey   string
		endpoint string
		peer     v1alpha1.WireguardPeer
	}

	type table struct {
		dns            string
		expectedOutput string
	}

	o := onpar.BeforeEach(onpar.New(t), func(t *testing.T) testContext {
		key, err := wgtypes.GenerateKey()
		assert.Nil(t, err)

		peer := dsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{
				Address: "192.168.1.2/24",
			}, v1alpha1.WireguardPeerStatus{
				PublicKey: toPtr("kekeke"),
			},
		)

		return testContext{
			privKey:  key.String(),
			pubKey:   key.PublicKey().String(),
			endpoint: "127.0.0.1:51820",
			peer:     peer,
		}
	})
	defer o.Run()

	o.Spec("errors if dns is not resolvable", func(tc testContext) {
		ep := tc.endpoint
		wg := dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			DNS: "not.exists",
		}, defaultWireguard.Status)
		fact := Peer{
			Scheme:    scheme,
			Peer:      tc.peer,
			Wireguard: wg,
		}
		secret, err := fact.Secret(ep, tc.pubKey, tc.privKey)
		assert.NotNil(t, err)
		assert.Nil(t, secret)
	})

	onpar.TableSpec(o, func(testCtx testContext, tt table) {
		ep := testCtx.endpoint
		wg := dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			DNS: tt.dns,
		}, defaultWireguard.Status)
		fact := Peer{
			Scheme:    scheme,
			Peer:      testCtx.peer,
			Wireguard: wg,
		}
		secret, err := fact.Secret(ep, testCtx.pubKey, testCtx.privKey)
		assert.Nil(t, err)
		assert.NotEmpty(t, secret.Data)
		assert.Contains(t, secret.Data, "config")

		config := string(secret.Data["config"])
		dns := fmt.Sprintf("DNS = %s\n", tt.expectedOutput)
		assert.Contains(t, config, dns)
	}).
		Entry("uses ip", table{"127.0.0.1", "127.0.0.1"}).
		Entry("resolves external url", table{"one.one.one.one", "1.0.0.1"})
}

func TestPeerDefaultConfigurations(t *testing.T) {
	t.Parallel()

	key, err := wgtypes.GenerateKey()
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

	key, err := wgtypes.GenerateKey()
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

	shouldHaveProperAnnotations(t, secret)
}
