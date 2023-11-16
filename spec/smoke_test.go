package spec

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

const (
	timeout  = 2 * time.Minute
	interval = 500 * time.Millisecond
)

func TestCRDsShouldBeInstalled(t *testing.T) {
	t.Parallel()

	client, err := makeApiExtensionsClient()
	assert.Nil(t, err, "k8s should be available")

	opts := metav1.GetOptions{}
	wg, err := client.ApiextensionsV1().CustomResourceDefinitions().
		Get(ctx, "wireguards.vpn.ahova.com", opts)
	assert.Nil(t, err)
	assert.NotEmpty(t, wg, "should have wireguard crd installed")

	peer, err := client.ApiextensionsV1().CustomResourceDefinitions().
		Get(ctx, "wireguardpeers.vpn.ahova.com", opts)
	assert.Nil(t, err)
	assert.NotEmpty(t, peer, "should have peer crd installed")
}

func TestSamplesShouldBeConnectable(t *testing.T) {
	t.Parallel()

	dsl, err := NewDsl(t)
	assert.Nil(t, err)
	assert.NotNil(t, dsl)

	err = dsl.MakeSamples()
	assert.Nil(t, err, "samples should be deployed")

	var peerConfig string
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		client := dsl.StaticClient.CoreV1().Secrets(namespace)
		opts := metav1.GetOptions{}
		secret, err := client.Get(ctx, "peer-sample", opts)
		assert.Nil(c, err, "should find secret")

		data := secret.Data
		assert.Contains(c, data, "config", "secret should have config")

		peerConfig = string(secret.Data["config"])
		assert.NotEmpty(c, peerConfig, "config should not be empty")
	}, timeout, interval, "should eventually produce a valid secret")
	t.Logf("config: %s\n", peerConfig)

	peer, err := dsl.SpawnPeer(peerConfig)
	assert.Nil(t, err)
	assert.NotEmpty(t, peer)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		output, err := dsl.Exec(peer.ID, []string{"wg"})
		assert.Nil(c, err)
		assert.NotEmpty(c, output)
		assert.Contains(c, output, "transfer:")
		t.Logf("wg output: %s\n", output)
	}, timeout, interval, "peer should be connectable")

	rmOpts := types.ContainerRemoveOptions{Force: true}
	err = dsl.DockerClient.ContainerRemove(ctx, peer.ID, rmOpts)
	assert.Nil(t, err, "container should be removed")

	err = dsl.DeleteSamples()
	assert.Nil(t, err, "samples should be deletable")
}
