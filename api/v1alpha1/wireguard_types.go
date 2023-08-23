package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WireguardSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	// +kubebuilder:validation:ExclusiveMaximum=false
	// +kubebuilder:default=1

	// Replicas defines the number of Wireguard instances
	Replicas int32 `json:"replicas,omitempty"`

	// Public address to the wireguard network
	EndpointAddress *string `json:"endpointAddress,omitempty"`

	// +kubebuilder:default="0.0.0.0/0, ::/0"

	// IP addresses allowed to be routed
	AllowedIPs string `json:"allowedIPs,omitempty"`

	// +kubebuilder:default="192.168.254.1/24"

	// Network space to use
	Network string `json:"network,omitempty"`

	// +kubebuilder:default={"192.168.0.0/16","172.16.0.0/12","10.0.0.0/8","169.254.169.254/32"}

	// Do not allow connections from peer to DropConnectionsTo IP addresses
	DropConnectionsTo []string `json:"dropConnectionsTo,omitempty"`

	// +kubebuilder:validation:Optional

	// DNS configuration for peer
	DNS *DNS `json:"dns,omitempty"`

	// Sidecar containers to run
	Sidecars []corev1.Container `json:"sidecars,omitempty"`

	// Affinity configuration
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// +kubebuilder:default="ClusterIP"

	// Type of the service to be created
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// Annotations for the service resource
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`
}

type DNS struct {
	// Indicates whether to use internal kubernetes dns
	DeployServer bool `json:"deployServer,omitempty"`

	// Address is an IPV4 address of the DNS server
	Address string `json:"address,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// WireguardPeer is the Schema for the wireguardpeers API
type Wireguard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WireguardSpec   `json:"spec,omitempty"`
	Status WireguardStatus `json:"status,omitempty"`
}

type WireguardStatus struct {
	PublicKey *string `json:"publicKey,omitempty"`
	Endpoint  *string `json:"endpoint,omitempty"`
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
