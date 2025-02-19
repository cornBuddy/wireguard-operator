package factory

import (
	"log"
	"os"
	"testing"

	"github.com/ahova/ahova-vpn/services/wireguard-operator/api/v1alpha1"
	"github.com/ahova/ahova-vpn/services/wireguard-operator/test/dsl"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/stretchr/testify/assert"
)

var (
	scheme *runtime.Scheme

	defaultWgFact   Wireguard
	defaultPeerFact Peer

	defaultWireguard = dsl.GenerateWireguard(v1alpha1.WireguardSpec{
		Address:     "192.168.1.1/24",
		ServiceType: corev1.ServiceTypeClusterIP,
		DNS:         "127.0.0.1",
	}, v1alpha1.WireguardStatus{
		Endpoint:  toPtr("127.0.0.1:51820"),
		PublicKey: toPtr("kekeke"),
	})
	defaultPeer = dsl.GeneratePeer(
		v1alpha1.WireguardPeerSpec{
			WireguardRef: defaultWireguard.GetName(),
			Address:      "192.168.1.2/24",
		}, v1alpha1.WireguardPeerStatus{PublicKey: toPtr("kekeke")},
	)
)

func TestMain(m *testing.M) {
	scheme = runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	if err != nil {
		log.Fatalf("cannot setup scheme: %v", err)
	}

	defaultWgFact = Wireguard{
		Scheme:    scheme,
		Wireguard: defaultWireguard,
		Peers: v1alpha1.WireguardPeerList{
			Items: []v1alpha1.WireguardPeer{defaultPeer},
		},
	}
	defaultPeerFact = Peer{
		Scheme:    scheme,
		Peer:      defaultPeer,
		Wireguard: defaultWireguard,
	}

	os.Exit(m.Run())
}

func shouldHaveProperAnnotations(t *testing.T, obj metav1.Object) {
	t.Helper()

	annotations := obj.GetAnnotations()
	assert.Contains(t, annotations, lastAppliedAnnotation)
	assert.NotEmpty(t, annotations[lastAppliedAnnotation])
	assert.Len(t, obj.GetOwnerReferences(), 1)
}
