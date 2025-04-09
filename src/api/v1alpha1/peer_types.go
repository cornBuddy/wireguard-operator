package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WireguardPeerSpec defines the desired state of Wireguard
type WireguardPeerSpec struct {
	// +kubebuilder:default="192.168.254.2/24"

	// IP address of the peer
	Address `json:"address,omitempty"`

	// +kubebuilder:validation:Required

	// Required. Reference to the wireguard resource
	WireguardRef string `json:"wireguardRef,omitempty"`

	// +kubebuilder:validation:MaxLength=44
	// +kubebuilder:validation:MinLength=44
	// +kubebuilder:example="WsFemZZdyC+ajbvOtKA7dltaNCaPOusKmkJffjMOMmg="

	// Public key of the peer
	PublicKey *string `json:"publicKey,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// WireguardPeer is the Schema for the wireguardpeers API
type WireguardPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WireguardPeerSpec   `json:"spec,omitempty"`
	Status WireguardPeerStatus `json:"status,omitempty"`
}

type WireguardPeerStatus struct {
	// Public key of the peer
	PublicKey *string `json:"publicKey,omitempty"`
}

//+kubebuilder:object:root=true

// WireguardPeerList contains a list of Wireguard Peer
type WireguardPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WireguardPeer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WireguardPeer{}, &WireguardPeerList{})
}
