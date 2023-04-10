package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WireguardPeerSpec defines the desired state of Wireguard
type WireguardPeerSpec struct {
	// +kubebuilder:validation:Required

	// Reference to the wireguard resource
	WireguardRef string `json:"wireguardRef,omitempty"`

	// DNS configuration for peer
	DNS DNS `json:"dns,omitempty"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	// +kubebuilder:validation:ExclusiveMaximum=false
	// +kubebuilder:default=1

	// Replicas defines the number of Wireguard instances
	Replicas int32 `json:"replicas,omitempty"`

	// Affinity configuration
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Public key of the peer. If the field is provided, it is assumed that
	// client peer is already configured, so no client config will be
	// defined in corresponding secret
	PublicKey *string `json:"publicKey,omitempty"`

	// +kubebuilder:default=51820

	// Port defines the port that will be used to init the container with
	// the image
	ContainerPort int32 `json:"containerPort,omitempty"`

	// +kubebuilder:default="192.168.254.2"

	// IP address of the peer
	Address string `json:"network,omitempty"`

	// +kubebuilder:default={"192.168.0.0/16","172.16.0.0/12","10.0.0.0/8","169.254.169.254/32"}

	// Do not allow connections from peer to DropConnectionsTo IP addresses
	DropConnectionsTo []string `json:"dropConnectionsTo,omitempty"`

	// Sidecar containers to run
	Sidecars []corev1.Container `json:"sidecars,omitempty"`
}

type DNS struct {
	// +kubebuilder:default=true

	// Indicates whether to use internal kubernetes dns
	DeployServer bool `json:"deployServer,omitempty"`

	// +kubebuilder:default="docker.io/klutchell/unbound:v1.17.1"

	// Image defines the image of the dns server
	Image string `json:"image,omitempty"`

	// +kubebuilder:default="192.168.254.1"

	// Address is an IPV4 address of the DNS server
	Address string `json:"address,omitempty"`
}

// WireguardPeerStatus defines the observed state of Wireguard
type WireguardPeerStatus struct {
	// Represents the observations of a Wireguard's current state.
	// Wireguard.status.conditions.type are: "Available", "Progressing", and
	// "Degraded"
	// Wireguard.status.conditions.status are one of True, False, Unknown.
	// Wireguard.status.conditions.reason the value should be a CamelCase
	// string and producers of specific condition types may define expected
	// values and meanings for this field, and whether the values are
	// considered a guaranteed API.
	// Wireguard.status.conditions.Message is a human readable message
	// indicating details about the transition.
	// For further information see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
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
