package controllers

import (
	"fmt"
	"testing"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/test/dsl"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/poy/onpar"
	"github.com/stretchr/testify/assert"
)

func TestPeerStatus(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	o.Spec("should set public key to status when explicitly defined in the spec", func(t *testing.T) {
		creds, err := wgtypes.GeneratePrivateKey()
		assert.Nil(t, err)

		wg := dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		pubKey := creds.PublicKey().String()
		peer := dsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
			PublicKey:    toPtr(pubKey),
		}, v1alpha1.WireguardPeerStatus{})

		assert.Eventually(t, func() bool {
			err := wgDsl.Apply(ctx, &wg)
			return err == nil
		}, timeout, tick)

		assert.Eventually(t, func() bool {
			err := peerDsl.Apply(ctx, &peer)
			return err == nil
		}, timeout, tick)

		key := types.NamespacedName{
			Namespace: peer.GetNamespace(),
			Name:      peer.GetName(),
		}
		assert.Eventually(t, func() bool {
			err := k8sClient.Get(ctx, key, &peer)
			return err == nil
		}, timeout, tick)

		assert.Equal(t, pubKey, *peer.Status.PublicKey)
	})

	o.Spec("should set public key to status when after it's written in the secret", func(t *testing.T) {
		wg := dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		peer := dsl.GeneratePeer(v1alpha1.WireguardPeerSpec{
			WireguardRef: wg.GetName(),
		}, v1alpha1.WireguardPeerStatus{})

		assert.Eventually(t, func() bool {
			err := wgDsl.Apply(ctx, &wg)
			return err == nil
		}, timeout, tick)

		assert.Eventually(t, func() bool {
			err := peerDsl.Apply(ctx, &peer)
			return err == nil
		}, timeout, tick)

		key := types.NamespacedName{
			Namespace: peer.GetNamespace(),
			Name:      peer.GetName(),
		}
		assert.Eventually(t, func() bool {
			err := k8sClient.Get(ctx, key, &peer)
			return err == nil
		}, timeout, tick)

		secret := &corev1.Secret{}
		assert.Eventually(t, func() bool {
			err := k8sClient.Get(ctx, key, secret)
			return err == nil
		}, timeout, tick)

		pubKey := string(secret.Data["public-key"])
		assert.Equal(t, pubKey, *peer.Status.PublicKey)
	})
}

func TestPeerEndpoint(t *testing.T) {
	t.Parallel()

	type testCase struct {
		description   string
		wireguard     v1alpha1.Wireguard
		wireguardPeer v1alpha1.WireguardPeer
		// function which can extract proper endpoitn from given service
		extractEndpoint endpointExtractor
	}

	testCases := []testCase{{
		description: "default resources",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		),
		wireguardPeer: dsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{},
			v1alpha1.WireguardPeerStatus{},
		),
		extractEndpoint: extractClusterIp,
	}, {
		description: "public ip not yet set",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{
				ServiceType: corev1.ServiceTypeLoadBalancer,
			},
			v1alpha1.WireguardStatus{},
		),
		wireguardPeer: dsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{},
			v1alpha1.WireguardPeerStatus{},
		),
		extractEndpoint: extractClusterIp,
	}, {
		description: "wireguard endpoint is set",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{
				EndpointAddress: toPtr("localhost"),
			},
			v1alpha1.WireguardStatus{},
		),
		wireguardPeer: dsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{},
			v1alpha1.WireguardPeerStatus{},
		),
		extractEndpoint: extractWireguardEndpoint,
	}}

	o := onpar.New(t)
	defer o.Run()

	spec := onpar.TableSpec(o, func(t *testing.T, test testCase) {
		// as those resources are anonymous at definition, it's required
		// to explicitly set wireguard reference for reconcile working
		// properly
		test.wireguardPeer.Spec.WireguardRef = test.wireguard.GetName()

		err := wgDsl.Apply(ctx, &test.wireguard)
		assert.Nil(t, err)

		err = peerDsl.Apply(ctx, &test.wireguardPeer)
		assert.Nil(t, err)

		secret := &corev1.Secret{}
		peerKey := types.NamespacedName{
			Name:      test.wireguardPeer.GetName(),
			Namespace: test.wireguardPeer.GetNamespace(),
		}
		err = k8sClient.Get(ctx, peerKey, secret)
		assert.Nil(t, err)
		assert.Contains(t, secret.Data, "config")
		assert.NotEmpty(t, secret.Data["config"])

		wgSvc := &corev1.Service{}
		wgKey := types.NamespacedName{
			Name:      test.wireguard.GetName(),
			Namespace: test.wireguard.GetNamespace(),
		}
		err = k8sClient.Get(ctx, wgKey, wgSvc)
		assert.Nil(t, err)

		cfg := string(secret.Data["config"])
		ep := test.extractEndpoint(test.wireguard, *wgSvc)
		epCfg := fmt.Sprintf("Endpoint = %s:%d\n", ep, wireguardPort)
		assert.Contains(t, cfg, epCfg)

	})

	for _, tc := range testCases {
		spec.Entry(tc.description, tc)
	}
}
