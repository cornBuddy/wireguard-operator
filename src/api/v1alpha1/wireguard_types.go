package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WireguardSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:validation:ExclusiveMaximum=false
	// +kubebuilder:default=1

	// Replicas defines the number of Wireguard instances
	Replicas int32 `json:"replicas,omitempty"`

	// +kubebuilder:default="ClusterIP"

	// Type of the service to be created
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// +kubebuilder:default="0.0.0.0/0"

	// IP addresses allowed to be routed
	AllowedIPs string `json:"allowedIPs,omitempty"`

	// +kubebuilder:default="192.168.254.1/24"

	// Address space to use
	Address `json:"address,omitempty"`

	// +kubebuilder:default="1.1.1.1"

	// DNS configuration for peer
	DNS string `json:"dns,omitempty"`

	// +kubebuilder:example="example.com:51820"

	// Address which going to be used in peers configuration. By default,
	// operator will use IP address of the service, which is not always
	// desirable (e.g. if public DNS record is attached to load balancer).
	// If port is not set, default wireguard port is used in status
	EndpointAddress *string `json:"endpointAddress,omitempty"`

	// Deny connections to the following list of IPs
	DropConnectionsTo []string `json:"dropConnectionsTo,omitempty"`

	// Sidecar containers to run
	Sidecars []corev1.Container `json:"sidecars,omitempty"`

	// Affinity configuration
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Annotations for the service resource
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`

	// Extra labels for all resources created
	Labels map[string]string `json:"labels,omitempty"`
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
	// Public key of the peer
	PublicKey *string `json:"publicKey,omitempty"`

	// Endpoint of the peer
	Endpoint *string `json:"endpoint,omitempty"`
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
