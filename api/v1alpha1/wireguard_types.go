package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WireguardSpec struct {
	// +kubebuilder:default="localhost"

	// Public address to the wireguard network
	EndpointAddress string `json:"endpointAddress,omitempty"`

	// +kubebuilder:default="0.0.0.0/0, ::/0"

	// IP addresses allowed to be routed
	AllowedIPs string `json:"allowedIPs,omitempty"`

	// +kubebuilder:default="192.168.254.1/24"

	// Network space to use
	Network string `json:"network,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// WireguardPeer is the Schema for the wireguardpeers API
type Wireguard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec WireguardSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// WireguardPeerList contains a list of Wireguard Peer
type WireguardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Wireguard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Wireguard{}, &WireguardList{})
}
