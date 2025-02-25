package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
	"github.com/cornbuddy/wireguard-operator/src/test/dsl"
)

func TestKeyGeneration(t *testing.T) {
	wg := dsl.GenerateWireguard(
		v1alpha1.WireguardSpec{},
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

	peerKey := types.NamespacedName{
		Name:      peer.GetName(),
		Namespace: peer.GetNamespace(),
	}
	peerSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, peerKey, peerSecret)
	assert.Nil(t, err)

	wgKey := types.NamespacedName{
		Name:      wg.GetName(),
		Namespace: wg.GetNamespace(),
	}
	wgSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, wgKey, wgSecret)
	assert.Nil(t, err)

	keys := []string{"config", "private-key", "public-key"}
	for _, key := range keys {
		assert.Contains(t, wgSecret.Data, key)
		assert.Contains(t, peerSecret.Data, key)
		assert.NotEqual(t, wgSecret.Data[key], peerSecret.Data[key],
			"keys should be different")
	}
}
