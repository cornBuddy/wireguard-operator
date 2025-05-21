package dsl

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
)

type Dsl struct {
	K8sClient client.Client
	reconcile.Reconciler
}

func (dsl Dsl) MakeWireguardWithSpec(ctx context.Context, spec v1alpha1.WireguardSpec) (*v1alpha1.Wireguard, error) {
	wg := GenerateWireguard(spec, v1alpha1.WireguardStatus{})
	if err := dsl.Apply(ctx, &wg); err != nil {
		return nil, err
	}

	key := types.NamespacedName{
		Namespace: wg.GetNamespace(),
		Name:      wg.GetName(),
	}
	if err := dsl.K8sClient.Get(ctx, key, &wg); err != nil {
		return nil, err
	}

	return &wg, nil
}

func (dsl Dsl) Reconcile(ctx context.Context, object client.Object) error {
	// Reconcile resource multiple times to ensure that all resources are
	// created
	const reconcilationLoops = 10
	key := types.NamespacedName{
		Name:      object.GetName(),
		Namespace: object.GetNamespace(),
	}
	req := reconcile.Request{NamespacedName: key}
	for i := 0; i < reconcilationLoops; i++ {
		if _, err := dsl.Reconciler.Reconcile(ctx, req); err != nil {
			return err
		}

		time.Sleep(1 * time.Second)
	}

	return nil
}

// Creates and reconciles given object. Updates object as a side effect
func (dsl Dsl) Apply(ctx context.Context, object client.Object) error {
	if err := dsl.K8sClient.Create(ctx, object); err != nil {
		return err
	}

	if err := dsl.Reconcile(ctx, object); err != nil {
		return err
	}

	key := types.NamespacedName{
		Name:      object.GetName(),
		Namespace: object.GetNamespace(),
	}
	if err := dsl.K8sClient.Get(ctx, key, object); err != nil {
		return err
	}

	return nil
}
