package spec

import (
	"context"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/poy/onpar"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go/modules/compose"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	testdsl "github.com/cornbuddy/wireguard-operator/spec/test/dsl"
)

const (
	timeout   = 8 * time.Minute
	tick      = 10 * time.Second
	namespace = "default"
)

func TestSamplesShouldBeConnectable(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
	})

	dsl, err := testdsl.NewDsl(ctx, t)
	assert.Nil(t, err)
	assert.NotNil(t, dsl)

	t.Log("Applying samples")
	err = dsl.ApplySamples(namespace)
	assert.Nil(t, err, "samples should be deployed")

	t.Cleanup(func() {
		t.Log("Deleting samples")
		err := dsl.DeleteSamples(namespace)
		assert.Nil(t, err, "samples should be deletable")
	})

	o := onpar.New(t)
	defer o.Run()

	type testCase struct {
		peer string
	}

	spec := onpar.TableSpec(o, func(t *testing.T, tc testCase) {
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			t.Log("Fetching peer secret")
			client := dsl.Clientset.CoreV1().Secrets(namespace)
			opts := metav1.GetOptions{}
			secret, err := client.Get(ctx, tc.peer, opts)
			assert.Nil(c, err, "should find secret")

			data := secret.Data
			assert.Contains(c, data, "config", "secret should have config")

			peerConfig := string(secret.Data["config"])
			assert.NotEmpty(c, peerConfig, "config should not be empty")

			ep := regexp.MustCompile("Endpoint = .+\\.amazonaws.com:51820")
			assert.Regexp(c, ep, peerConfig)

			t.Log("Provisioning docker compose stack for wireguard peer")
			peer, err := dsl.StartPeerWithConfig(peerConfig)
			assert.Nil(c, err)
			assert.NotNil(c, peer, "should create stack for peer")

			// it's expected for peer to be nil when StartPeerWithConfig is
			// failed, so returning early to avoid panic
			if peer == nil {
				return
			}

			t.Log("Fetching peer container")
			container, err := peer.ServiceContainer(ctx, testdsl.PeerServiceName)
			assert.Nil(c, err)
			assert.NotEmpty(c, container, "should start peer container")

			type validateCommandOutputTestCase struct {
				message  string
				command  []string
				contains string
			}

			tcs := []validateCommandOutputTestCase{{
				message:  "Checking DNS connectivity",
				command:  []string{"nslookup", "google.com"},
				contains: "Name:\tgoogle.com",
			}, {
				message:  "Checking ICMP connectivity",
				command:  []string{"ping", "-c", "4", "8.8.8.8"},
				contains: "4 packets transmitted, 4 received",
			}, {
				message:  "Checking internet connectivity",
				command:  []string{"curl", "-v", "google.com"},
				contains: "301 Moved",
			}, {
				message:  "Checking wireguard data transfer",
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
				assert.Nil(c, err)

				output := string(bytes)
				assert.NotEmpty(c, output)
				assert.Contains(c, output, tc.contains)

				t.Logf("### `%v` output: %s", tc.command, output)
			}

			t.Log("Tearing stack down")
			err = peer.Down(ctx, compose.RemoveOrphans(true))
			assert.Nil(t, err, "should stop peer")
		}, timeout, tick, "peer should be connectable")
	})

	dri := dsl.DynamicClient.Resource(testdsl.PeerGvr).Namespace(namespace)
	opts := metav1.ListOptions{}
	peersList, err := dri.List(ctx, opts)
	assert.Nil(t, err, "should find some peers")

	for _, peer := range peersList.Items {
		tc := testCase{peer: peer.GetName()}
		spec.Entry(peer.GetName(), tc)
	}
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
	opts := metav1.GetOptions{}
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

func TestCRDsShouldBeInstalled(t *testing.T) {
	t.Parallel()

	client, err := testdsl.MakeApiExtensionsClient()
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
