package dsl

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler interface {
	Reconcile(context.Context, ctrl.Request) (ctrl.Result, error)
	Status() client.SubResourceWriter
}

type Dsl struct {
	K8sClient client.Client
	Reconciler
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
