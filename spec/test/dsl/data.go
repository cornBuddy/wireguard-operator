package dsl

import (
	"context"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	PeerServiceName = "peer"
)

var (
	WireguardGvr = schema.GroupVersionResource{
		Group:    "vpn.ahova.com",
		Version:  "v1alpha1",
		Resource: "wireguards",
	}
	PeerGvr = schema.GroupVersionResource{
		Group:    "vpn.ahova.com",
		Version:  "v1alpha1",
		Resource: "wireguardpeers",
	}
)

type Dsl struct {
	Clientset     *kubernetes.Clientset
	DynamicClient *dynamic.DynamicClient

	apiExtensionsClient *clientset.Clientset
	ctx                 context.Context
	t                   *testing.T
}

type Spec map[string]interface{}
