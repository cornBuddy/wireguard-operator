package acceptance

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/poy/onpar"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	testdsl "github.com/cornbuddy/wireguard-operator/spec/test/dsl"
)

const (
	timeout   = 15 * time.Minute
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
		var peerConfig string
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			t.Log("Fetching peer secret")
			client := dsl.Clientset.CoreV1().Secrets(namespace)
			opts := v1.GetOptions{}
			secret, err := client.Get(ctx, tc.peer, opts)
			assert.Nil(c, err, "should find secret")

			data := secret.Data
			assert.Contains(c, data, "config", "secret should have config")

			peerConfig = string(secret.Data["config"])
			assert.NotEmpty(c, peerConfig, "config should not be empty")

			ep := regexp.MustCompile("Endpoint = .+\\.amazonaws.com:51820")
			assert.Regexp(c, ep, peerConfig)
		}, timeout, tick)

		t.Log("Asserting peer connectivity")
		dsl.AssertPeerIsEventuallyConnectable(tc.peer, peerConfig, timeout, tick)
	})

	dri := dsl.DynamicClient.Resource(testdsl.PeerGvr).Namespace(namespace)
	opts := v1.ListOptions{}
	peersList, err := dri.List(ctx, opts)
	assert.Nil(t, err, "should find some peers")

	for _, peer := range peersList.Items {
		tc := testCase{peer: peer.GetName()}
		spec.Entry(peer.GetName(), tc)
	}
}
