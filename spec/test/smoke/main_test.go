package smoke

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	testdsl "github.com/cornbuddy/wireguard-operator/spec/test/dsl"
)

const (
	timeout   = 5 * time.Minute
	tick      = 10 * time.Second
	namespace = "default"
)

func TestCRDsShouldBeInstalled(t *testing.T) {
	t.Parallel()

	client, err := testdsl.MakeApiExtensionsClient()
	assert.Nil(t, err, "k8s should be available")

	ctx := context.TODO()
	opts := v1.GetOptions{}
	wg, err := client.ApiextensionsV1().CustomResourceDefinitions().
		Get(ctx, "wireguards.vpn.ahova.com", opts)
	assert.Nil(t, err)
	assert.NotEmpty(t, wg, "should have wireguard crd installed")

	peer, err := client.ApiextensionsV1().CustomResourceDefinitions().
		Get(ctx, "wireguardpeers.vpn.ahova.com", opts)
	assert.Nil(t, err)
	assert.NotEmpty(t, peer, "should have peer crd installed")
}

func TestWirguardSecretIsUpdatedWhenPeerListChanges(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		t.Log("Cancelling context")
		cancel()
	})

	dsl, err := testdsl.NewDsl(ctx, t)
	assert.Nil(t, err)
	assert.NotNil(t, dsl)

	t.Log("Creating wireguard resource")
	spec := map[string]interface{}{
		"replicas": 1,
	}
	wgName, err := dsl.CreateWireguardWithSpec(namespace, spec)
	assert.Nil(t, err, "should create default wireguard instance")
	t.Cleanup(func() {
		t.Log("Deleting wireguard")
		err := dsl.DeleteWireguard(wgName, namespace)
		assert.Nil(t, err)
	})

	t.Log("Fetching wireguard secret")
	secretClient := dsl.Clientset.CoreV1().Secrets(namespace)
	opts := v1.GetOptions{}
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		secret, err := secretClient.Get(ctx, wgName, opts)
		assert.Nil(c, err, "should find secret")
		assert.NotContains(c, string(secret.Data["config"]), "[Peer]")
	}, timeout, tick, "should start without any [Peer]'s")

	t.Log("Creating peer resource")
	spec = map[string]interface{}{
		"wireguardRef": wgName,
	}
	peerName, err := dsl.CreatePeerWithSpec(namespace, spec)
	assert.Nil(t, err, "should create default wireguard instance")

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		t.Log("Fetching wireguard secret")
		secret, err := secretClient.Get(ctx, wgName, opts)
		assert.Nil(c, err, "should find secret")
		assert.Contains(c, string(secret.Data["config"]), "[Peer]")
	}, timeout, tick, "[Peer] should pop up once peer is added")

	t.Log("Deleting peer")
	err = dsl.DeletePeer(peerName, namespace)
	assert.Nil(t, err)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		t.Log("Fetching wireguard secret")
		secret, err := secretClient.Get(ctx, wgName, opts)
		assert.Nil(c, err, "should find secret")
		assert.NotContains(c, string(secret.Data["config"]), "[Peer]")
	}, timeout, tick, "[Peer] should begone once peer is removed")
}
