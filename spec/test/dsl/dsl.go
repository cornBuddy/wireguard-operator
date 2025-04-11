package dsl

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	network = "wireguard-operator-spec"
)

// Set's up wireguard peer and tries to connect to it
func (dsl Dsl) AssertPeerIsEventuallyConnectable(
	name, peerConfig string,
	timeout, tick time.Duration) {

	t := dsl.t
	ctx := dsl.ctx
	log := func(message string) {
		msg := fmt.Sprintf("[%s] %s", name, message)
		t.Log(msg)
	}
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		log("Provisioning docker compose stack for wireguard peer")
		peer, err := dsl.StartPeerWithConfig(peerConfig)
		assert.Nil(c, err)
		assert.NotNil(c, peer, "should create stack for peer")

		// it's expected for peer to be nil when StartPeerWithConfig is
		// failed, so returning early to avoid panic
		if peer == nil {
			return
		}

		log("Fetching peer container")
		container, err := peer.ServiceContainer(ctx, PeerServiceName)
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

		log("Tearing stack down")
		err = peer.Down(ctx, compose.RemoveOrphans(true))
		assert.Nil(t, err, "should stop peer")
	}, timeout, tick, "peer should be connectable")
}

func (dsl Dsl) CreatePeerWithSpec(namespace string, spec Spec) (string, error) {
	name := randomString()
	obj := map[string]interface{}{
		"apiVersion": "vpn.ahova.com/v1alpha1",
		"kind":       "WireguardPeer",
		"metadata": map[string]string{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}
	unstrcd := &unstructured.Unstructured{
		Object: obj,
	}

	opts := v1.CreateOptions{}
	dri := dsl.DynamicClient.Resource(PeerGvr).Namespace(namespace)
	if _, err := dri.Create(dsl.ctx, unstrcd, opts); err != nil {
		return "", err
	}

	return name, nil

}

func (dsl Dsl) DeletePeer(name, namespace string) error {
	opts := v1.DeleteOptions{}
	dri := dsl.DynamicClient.Resource(PeerGvr).Namespace(namespace)
	if err := dri.Delete(dsl.ctx, name, opts); err != nil {
		return err
	}

	return nil
}

func (dsl Dsl) CreateWireguardWithSpec(
	namespace string, spec Spec) (
	string, error) {

	if spec["serviceAnnotations"] == nil {
		spec["serviceAnnotations"] = map[string]interface{}{}
	}

	// annotations below are required for eks, and I don't want to copypaste
	// them in each procedure call
	defaultServiceAnnotations := map[string]interface{}{
		"service.beta.kubernetes.io/aws-load-balancer-type":                              "nlb",
		"service.beta.kubernetes.io/aws-load-balancer-backend-protocol":                  "udp",
		"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
		"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                  "10250",
		"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":              "tcp",
	}
	svcAnnotations := spec["serviceAnnotations"].(map[string]interface{})
	maps.Copy(svcAnnotations, defaultServiceAnnotations)
	spec["serviceAnnotations"] = svcAnnotations

	name := randomString()
	obj := map[string]interface{}{
		"apiVersion": "vpn.ahova.com/v1alpha1",
		"kind":       "Wireguard",
		"metadata": map[string]string{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}
	unstrcd := &unstructured.Unstructured{
		Object: obj,
	}

	opts := v1.CreateOptions{}
	dri := dsl.DynamicClient.Resource(WireguardGvr).Namespace(namespace)
	if _, err := dri.Create(dsl.ctx, unstrcd, opts); err != nil {
		return "", err
	}

	return name, nil
}

func (dsl Dsl) DeleteWireguard(name, namespace string) error {
	opts := v1.DeleteOptions{}
	dri := dsl.DynamicClient.Resource(WireguardGvr).Namespace(namespace)
	if err := dri.Delete(dsl.ctx, name, opts); err != nil {
		return err
	}

	return nil
}

func (dsl Dsl) StartPeerWithConfig(peerConfig string) (
	compose.ComposeStack, error) {

	c := fmt.Sprintf(
		"docker network inspect %s || docker network create %s",
		network, network,
	)
	cmd := exec.Command("bash", "-c", c)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	configPath, err := dsl.makeTempConfig(peerConfig)
	if err != nil {
		return nil, err
	}

	composePath, err := dsl.makeTempComposeFile(configPath, network)
	if err != nil {
		return nil, err
	}

	peerCompose, err := compose.NewDockerCompose(composePath)
	if err != nil {
		return nil, err
	}

	waitForWg := wait.ForExec(
		[]string{"/bin/bash", "-c", "wg"},
	).
		WithExitCodeMatcher(func(code int) bool {
			return code == 0
		})
	waits := []wait.Strategy{
		wait.ForLog("resolvconf -a wg0 -m 0 -x").AsRegexp(),
		waitForWg,
	}
	stack := peerCompose.WaitForService(
		PeerServiceName,
		wait.ForAll(waits...).WithDeadline(3*time.Second),
	)
	if err := stack.Up(
		dsl.ctx,
		compose.Wait(true),
		compose.RemoveOrphans(true),
	); err != nil {
		return nil, err
	}

	return stack, nil
}

func (dsl Dsl) ApplySamples(namespace string) error {
	if err := dsl.kustomizeSamples("apply"); err != nil {
		return err
	}

	return nil
}

func (dsl Dsl) DeleteSamples(namespace string) error {
	if err := dsl.kustomizeSamples("delete"); err != nil {
		return err
	}

	return nil
}

func NewDsl(ctx context.Context, t *testing.T) (*Dsl, error) {
	apiExtClient, err := MakeApiExtensionsClient()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := makeDynamicClient()
	if err != nil {
		return nil, err
	}

	staticClient, err := makeStaticClient()
	if err != nil {
		return nil, err
	}

	return &Dsl{
		Clientset:           staticClient,
		apiExtensionsClient: apiExtClient,
		DynamicClient:       dynamicClient,
		ctx:                 ctx,
		t:                   t,
	}, nil
}

func MakeApiExtensionsClient() (*clientset.Clientset, error) {
	kubeConfig, err := makeKubeConfig()
	if err != nil {
		return nil, err
	}

	client, err := clientset.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}
