package factory

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var (
	scheme *runtime.Scheme

	wireguardFactory Wireguard
	peerFactory      Peer

	defaultWireguard = testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
		ServiceType: corev1.ServiceTypeClusterIP,
	}, v1alpha1.WireguardStatus{
		Endpoint:  toPtr("127.0.0.1:51820"),
		PublicKey: toPtr("kekeke"),
	})
	defaultPeer = testdsl.GeneratePeer(
		v1alpha1.WireguardPeerSpec{
			WireguardRef: defaultWireguard.GetName(),
		}, v1alpha1.WireguardPeerStatus{PublicKey: toPtr("kekeke")},
	)
)

func TestFactory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "factory")
}

var _ = BeforeSuite(func() {
	scheme = runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())

	wireguardFactory = Wireguard{
		Scheme:    scheme,
		Wireguard: defaultWireguard,
		Peers: v1alpha1.WireguardPeerList{
			Items: []v1alpha1.WireguardPeer{defaultPeer},
		},
	}
	peerFactory = Peer{
		Scheme:    scheme,
		Peer:      defaultPeer,
		Wireguard: defaultWireguard,
	}
})

func shouldHaveProperDecorations(obj metav1.Object) {
	annotations := obj.GetAnnotations()
	Expect(annotations).To(HaveKey(lastAppliedAnnotation))
	Expect(annotations[lastAppliedAnnotation]).ToNot(BeEmpty())
	Expect(obj.GetOwnerReferences()).To(HaveLen(1))
}
