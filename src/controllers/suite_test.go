package controllers

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
	"github.com/cornbuddy/wireguard-operator/src/test/dsl"
	myEnvtest "github.com/cornbuddy/wireguard-operator/src/test/envtest"
	//+kubebuilder:scaffold:imports
)

const (
	timeout       = 10 * time.Second
	tick          = 1 * time.Second
	wireguardPort = 51820
)

var (
	config    *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	wgDsl     dsl.Dsl
	peerDsl   dsl.Dsl
	ctx       = context.TODO()
)

func TestMain(m *testing.M) {
	cfg, cleanup, err := myEnvtest.SetupEnvtest()
	if err != nil {
		log.Fatalf("failed to setup envtest: %v", err)
	}

	config = cfg

	defer (func() {
		if err := cleanup(); err != nil {
			log.Fatalf("failed to cleanup envtest: %v", err)
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
