package controllers

import (
	"fmt"
	"testing"

	"github.com/poy/onpar"
	"github.com/stretchr/testify/assert"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
	"github.com/cornbuddy/wireguard-operator/src/test/dsl"
)

func TestPeerEndpoint(t *testing.T) {
	t.Parallel()

	type testCase struct {
		description   string
		wireguard     v1alpha1.Wireguard
		wireguardPeer v1alpha1.WireguardPeer
		// function which can extract proper endpoint from given service
		extractEndpoint endpointExtractor
	}

	// testCases := []testCase{{
	// 	description: "default resources",
	// 	wireguard: dsl.GenerateWireguard(
	// 		v1alpha1.WireguardSpec{},
	// 		v1alpha1.WireguardStatus{},
	// 	),
	// 	wireguardPeer: dsl.GeneratePeer(
	// 		v1alpha1.WireguardPeerSpec{},
	// 		v1alpha1.WireguardPeerStatus{},
	// 	),
	// 	extractEndpoint: extractClusterIp,
	// }, {
	// 	description: "endpoint is set in status",
	// 	wireguard: dsl.GenerateWireguard(
	// 		v1alpha1.WireguardSpec{
	// 			ServiceType: corev1.ServiceTypeLoadBalancer,
	// 		},
	// 		v1alpha1.WireguardStatus{
	// 			Endpoint: toPtr("localhost"),
	// 		},
	// 	),
	// 	wireguardPeer: dsl.GeneratePeer(
	// 		v1alpha1.WireguardPeerSpec{},
	// 		v1alpha1.WireguardPeerStatus{},
	// 	),
	// 	extractEndpoint: extractFromStatus,
	// }, {
	// 	description: "wireguard endpoint is set",
	// 	wireguard: dsl.GenerateWireguard(
	// 		v1alpha1.WireguardSpec{
	// 			EndpointAddress: toPtr("localhost"),
	// 		},
	// 		v1alpha1.WireguardStatus{
	// 			Endpoint: toPtr("localhost"),
	// 		},
	// 	),
	// 	wireguardPeer: dsl.GeneratePeer(
	// 		v1alpha1.WireguardPeerSpec{},
	// 		v1alpha1.WireguardPeerStatus{},
	// 	),
	// 	extractEndpoint: extractWireguardEndpoint,
	// }}

	// NOTE: this test case is broken because service with type load
	// balancer cannot be created properly in envtest
	testCases := []testCase{{
		description: "endpoint is set in status",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{
				ServiceType: corev1.ServiceTypeLoadBalancer,
			},
			v1alpha1.WireguardStatus{
				Endpoint:  toPtr("localhost"),
				PublicKey: toPtr("kekeke"),
			},
		),
		wireguardPeer: dsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{},
			v1alpha1.WireguardPeerStatus{},
		),
		extractEndpoint: extractFromStatus,
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

		svc := &corev1.Service{}
		wgKey := types.NamespacedName{
			Name:      test.wireguard.GetName(),
			Namespace: test.wireguard.GetNamespace(),
		}
		err = k8sClient.Get(ctx, wgKey, svc)
		assert.Nil(t, err)

		cfg := string(secret.Data["config"])
		ep := test.extractEndpoint(test.wireguard, *svc)
		epCfg := fmt.Sprintf("Endpoint = %s:%d\n", ep, wireguardPort)
		assert.Contains(t, cfg, epCfg)
	})

	for _, tc := range testCases {
		spec.Entry(tc.description, tc)
	}

	// o.Spec("should not reconcile when service type is LB and endpoint is not set in status", func(t *testing.T) {
	// 	wg := dsl.GenerateWireguard(
	// 		v1alpha1.WireguardSpec{ServiceType: corev1.ServiceTypeLoadBalancer},
	// 		v1alpha1.WireguardStatus{},
	// 	)
	// 	err := wgDsl.Apply(ctx, &wg)
	// 	assert.Nil(t, err)

	// 	peer := dsl.GeneratePeer(
	// 		v1alpha1.WireguardPeerSpec{WireguardRef: wg.GetName()},
	// 		v1alpha1.WireguardPeerStatus{},
	// 	)
	// 	err = wgDsl.Apply(ctx, &peer)
	// 	assert.Nil(t, err)

	// 	key := types.NamespacedName{
	// 		Name:      peer.GetName(),
	// 		Namespace: peer.GetNamespace(),
	// 	}
	// 	err = k8sClient.Get(ctx, key, &peer)
	// 	assert.NotNil(t, err)
	// })
}

func TestPeerSecret(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	o.Spec("should resolve external dns", func(t *testing.T) {
		dns := "one.one.one.one"
		wg := dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{DNS: dns},
			v1alpha1.WireguardStatus{},
		)
		err := wgDsl.Apply(ctx, &wg)
		assert.Nil(t, err)

		peer := dsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{WireguardRef: wg.GetName()},
			v1alpha1.WireguardPeerStatus{},
		)
		err = peerDsl.Apply(ctx, &peer)
		assert.Nil(t, err)

		secret := &corev1.Secret{}
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			key := types.NamespacedName{
				Name:      peer.GetName(),
				Namespace: peer.GetNamespace(),
			}
			err := k8sClient.Get(ctx, key, secret)
			assert.Nil(c, err)
			assert.Contains(c, secret.Data, "config")
		}, timeout, tick)

		config := string(secret.Data["config"])
		assert.Contains(t, config, "DNS = 1.0.0.1\n")
	})
}

func TestPeerStatus(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	type testCase struct {
		description string
		peerSpec    v1alpha1.WireguardPeerSpec
	}

	creds, err := wgtypes.GeneratePrivateKey()
	assert.Nil(t, err)

	testCases := []testCase{{
		description: "should set pub key if defined in spec",
		peerSpec: v1alpha1.WireguardPeerSpec{
			PublicKey: toPtr(creds.PublicKey().String()),
		},
	}, {
		description: "should set pub key if not defined in spec",
		peerSpec:    v1alpha1.WireguardPeerSpec{},
	}}

	table := onpar.TableSpec(o, func(t *testing.T, tc testCase) {
		wg := dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		assert.Eventually(t, func() bool {
			err := wgDsl.Apply(ctx, &wg)
			return err == nil
		}, timeout, tick)

		tc.peerSpec.WireguardRef = wg.GetName()
		peer := dsl.GeneratePeer(
			tc.peerSpec,
			v1alpha1.WireguardPeerStatus{},
		)
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

	for _, tc := range testCases {
		table.Entry(tc.description, tc)
	}
}
