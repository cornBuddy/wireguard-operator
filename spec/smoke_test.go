package spec

import (
	"context"
	"io"
	"regexp"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go/modules/compose"
)

const (
	timeout = 5 * time.Minute
	tick    = 10 * time.Second
)

func TestSamplesShouldBeConnectable(t *testing.T) {
	t.Parallel()

	dsl, err := NewDsl(t)
	assert.Nil(t, err)
	assert.NotNil(t, dsl)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		t.Log("Deleting samples")
		err := dsl.DeleteSamples(ctx)
		assert.Nil(t, err, "samples should be deletable")

		t.Log("Cancelling peer context")
		cancel()
	})

	t.Log("Applying samples")
	err = dsl.ApplySamples(ctx)
	assert.Nil(t, err, "samples should be deployed")

	var peerConfig string
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		t.Log("Fetching peer secret")
		client := dsl.StaticClient.CoreV1().Secrets(namespace)
		opts := metav1.GetOptions{}
		secret, err := client.Get(ctx, "peer-sample", opts)
		assert.Nil(c, err, "should find secret")

		data := secret.Data
		assert.Contains(c, data, "config", "secret should have config")

		peerConfig = string(secret.Data["config"])
		assert.NotEmpty(c, peerConfig, "config should not be empty")

		endpointRegex := regexp.
			MustCompile("Endpoint = .+\\.amazonaws.com:51820")
		assert.Regexp(c, endpointRegex, peerConfig)
	}, timeout, tick, "should eventually produce a valid secret")

	var peer compose.ComposeStack
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		t.Log("Provisioning docker compose stack for wireguard peer")
		peer, err = dsl.StartPeerWithConfig(ctx, peerConfig)
		assert.Nil(c, err)
		assert.NotNil(c, peer, "should create stack for peer")

		// it's expected for peer to be nil when StartPeerWithConfig is
		// failed, so returning early to avoid panic
		if peer == nil {
			return
		}

		t.Log("Fetching peer container")
		container, err := peer.ServiceContainer(ctx, peerServiceName)
		assert.Nil(c, err)
		assert.NotEmpty(c, container, "should start peer container")

		type validateCommandOutputTestCase struct {
			message  string
			command  []string
			contains string
		}

		tcs := []validateCommandOutputTestCase{{
			message:  "Checking internet connectivity",
			command:  []string{"curl", "-Ss", "https://google.com"},
			contains: "301 Moved",
		}, {
			message:  "Executing `wg` command inside peer container",
			command:  []string{"wg"},
			contains: "KiB",
		}}

		for _, tc := range tcs {
			t.Logf("%s: executing `%v`", tc.message, tc.command)
			code, reader, err := container.Exec(ctx, tc.command)
			assert.Nil(c, err)
			assert.Equal(c, 0, code, "check should succeed")
			assert.NotEmpty(c, reader, "output should not be empty")

			if reader == nil {
				return
			}

			t.Logf("validating output of `%v`", tc.command)
			bytes, err := io.ReadAll(reader)
			output := string(bytes)
			assert.Nil(c, err)
			assert.NotEmpty(c, output)
			assert.Contains(c, output, tc.contains)

			t.Logf("### `%v` output: %s", tc.command, output)
		}
	}, timeout, tick, "should eventually setup wireguard peer")

	t.Cleanup(func() {
		t.Log("Tearing stack down")
		err := peer.Down(ctx, compose.RemoveOrphans(true))
		assert.Nil(t, err, "should stop peer")
	})
}

func TestCRDsShouldBeInstalled(t *testing.T) {
	t.Parallel()

	client, err := makeApiExtensionsClient()
	assert.Nil(t, err, "k8s should be available")

	ctx := context.TODO()
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
