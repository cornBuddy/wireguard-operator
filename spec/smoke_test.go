package spec

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

const (
	timeout  = 10 * time.Second
	interval = 200 * time.Millisecond
)

func TestCRDsShouldBeInstalled(t *testing.T) {
	t.Parallel()

	client, err := MakeApiExtensionsClient()
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

	apiExtClient, err := MakeApiExtensionsClient()
	assert.Nil(t, err, "k8s should be available")

	dynamicClient, err := MakeDynamicClient()
	assert.Nil(t, err, "k8s should be available")

	staticClient, err := MakeStaticClient()
	assert.Nil(t, err, "k8s should be available")

	dsl := Dsl{
		ApiExtensionsClient: apiExtClient,
		DynamicClient:       dynamicClient,
		StaticClient:        staticClient,
	}
	err = dsl.MakeSamples()
	assert.Nil(t, err, "samples should be deployed")

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		client := staticClient.CoreV1().Secrets(namespace)
		opts := metav1.GetOptions{}
		secret, err := client.Get(ctx, "peer-sample", opts)
		assert.Nil(c, err, "should find secret")
		assert.Contains(c, secret.Data, "config",
			"peer secret should have config")
	}, timeout, interval,
		"should eventually produce a secret with peer config")

	// TODO: ensure that peer is connectable

	err = dsl.DeleteSamples()
	assert.Nil(t, err, "samples should be deleted")
}
