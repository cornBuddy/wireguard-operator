package spec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/stretchr/testify/assert"
)

var (
	ctx = context.TODO()
)

const (
	timeout  = 10 * time.Second
	interval = 200 * time.Millisecond
)

func TestSamples(t *testing.T) {
	client, err := makeK8sClient()
	assert.Nil(t, err, "k8s config should be available")

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

func makeK8sClient() (*clientset.Clientset, error) {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	kubeConfigPath := filepath.Join(userHomeDir, ".kube", "config")
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, err
	}

	clientset, err := clientset.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}
