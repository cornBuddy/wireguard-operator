package dsl

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/storage/names"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
)

func GenerateWireguard(
	spec v1alpha1.WireguardSpec, status v1alpha1.WireguardStatus,
) v1alpha1.Wireguard {

	name := names.SimpleNameGenerator.GenerateName("wireguard-")
	return v1alpha1.Wireguard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: corev1.NamespaceDefault,
		},
		Spec:   spec,
		Status: status,
	}
}

func GeneratePeer(
	spec v1alpha1.WireguardPeerSpec, status v1alpha1.WireguardPeerStatus,
) v1alpha1.WireguardPeer {

	name := names.SimpleNameGenerator.GenerateName("peer-")
	return v1alpha1.WireguardPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: corev1.NamespaceDefault,
		},
		Spec:   spec,
		Status: status,
	}
}
