package controllers

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
	"github.com/cornbuddy/wireguard-operator/src/test/dsl"
	//+kubebuilder:scaffold:imports
)

const (
	timeout       = 10 * time.Second
	tick          = 1 * time.Second
	wireguardPort = 51820
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	wgDsl     dsl.Dsl
	peerDsl   dsl.Dsl
	ctx       = context.TODO()
)

func TestMain(m *testing.M) {
	crdPath := filepath.Join("..", "config", "crd", "bases")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		log.Fatalf("failed to start testenv: %v", err)
	}

	defer (func() {
		if err := testEnv.Stop(); err != nil {
			log.Fatalf("failed to stop envtest: %v", err)
		}
	})()

	err = v1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		log.Fatalf("failed to setup scheme: %v", err)
	}

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		log.Fatalf("failed to setup k8s client: %v", err)
	}

	peerDsl = dsl.Dsl{
		K8sClient: k8sClient,
		Reconciler: &WireguardPeerReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		},
	}
	wgDsl = dsl.Dsl{
		K8sClient: k8sClient,
		Reconciler: &WireguardReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		},
	}

	os.Exit(m.Run())
}
