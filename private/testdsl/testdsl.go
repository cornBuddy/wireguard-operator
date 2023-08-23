package testdsl

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

type Reconciler interface {
	Reconcile(context.Context, ctrl.Request) (ctrl.Result, error)
	Status() client.SubResourceWriter
}

type Dsl struct {
	K8sClient client.Client
	Reconciler
}

func GenerateWireguard(spec v1alpha1.WireguardSpec, status v1alpha1.WireguardStatus) v1alpha1.Wireguard {
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

func GeneratePeer(spec v1alpha1.WireguardPeerSpec, status v1alpha1.WireguardPeerStatus) v1alpha1.WireguardPeer {
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

func (dsl Dsl) Reconcile(object client.Object) error {
	// Reconcile resource multiple times to ensure that all resources are
	// created
	const reconcilationLoops = 10
	key := types.NamespacedName{
		Name:      object.GetName(),
		Namespace: object.GetNamespace(),
	}
	req := reconcile.Request{NamespacedName: key}
	ctx := context.TODO()
	for i := 0; i < reconcilationLoops; i++ {
		if _, err := dsl.Reconciler.Reconcile(ctx, req); err != nil {
			return err
		}
	}

	return nil
}

func (dsl Dsl) Apply(ctx context.Context, object client.Object) error {
	if err := dsl.K8sClient.Create(ctx, object); err != nil {
		return err
	}

	if err := dsl.Reconcile(object); err != nil {
		return err
	}

	return nil
}
