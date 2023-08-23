package controllers

import (
	"context"

	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/factory"
)

const (
	wireguardRefField = ".spec.wireguardRef"
)

// WireguardPeerReconciler reconciles a Wireguard object
type WireguardPeerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguardpeers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguardpeers/status,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguards,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguards/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch

func (r *WireguardPeerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).
		WithName("wireguard-peer").
		WithValues("Name", req.Name, "Namespace", req.Namespace)

	peer := &v1alpha1.WireguardPeer{}
	err := r.Get(ctx, req.NamespacedName, peer)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get wireguard peer")
		return ctrl.Result{}, err
	} else if apierrors.IsNotFound(err) {
		log.Info("wireguardpeer resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}
	log.Info("Successfully read peer from cluster, moving on...")

	wireguard := &v1alpha1.Wireguard{}
	wgKey := types.NamespacedName{
		Name:      peer.Spec.WireguardRef,
		Namespace: peer.GetNamespace(),
	}
	if err := r.Get(ctx, wgKey, wireguard); err != nil {
		log.Error(err, "Cannot retrieve parent wireguard resource")
		return ctrl.Result{}, nil
	}
	log.Info("Retrieved parent wireguard resource, moving on...")

	wgPubKey := wireguard.Status.PublicKey
	wgEndpoint := wireguard.Status.Endpoint
	if wgPubKey == nil || wgEndpoint == nil {
		log.Info("Corresponding wireguard is not yet reconciled",
			"WireguardRef", peer.Spec.WireguardRef,
			"Wireguard.Status", wireguard.Status)
		return ctrl.Result{}, nil
	}

	fact := factory.Peer{
		Scheme:    r.Scheme,
		Wireguard: *wireguard,
		Peer:      *peer,
	}
	secret, err := fact.Secret(*wgEndpoint)
	if err != nil {
		log.Error(err, "Cannot generate secret")
		return ctrl.Result{}, err
	}
	if created, err := r.createIfNotExists(peer, ctx, secret); err != nil {
		log.Error(err, "Cannot create secret")
		return ctrl.Result{}, err
	} else if created {
		log.Info("Secret created successfully")
		return ctrl.Result{}, nil
	}
	log.Info("Secret is created already, moving on...")

	if peer.Spec.PublicKey == nil {
		pubKey := string(secret.Data["public-key"])
		peer.Status.PublicKey = &pubKey
	} else {
		peer.Status.PublicKey = peer.Spec.PublicKey
	}

	if err := r.Status().Update(ctx, peer); err != nil {
		log.Error(err, "Cannot update status")
		return ctrl.Result{}, err
	}
	log.Info("Status is updated")

	return ctrl.Result{}, nil
}

func (r *WireguardPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.TODO()
	peer := &v1alpha1.WireguardPeer{}
	if err := mgr.GetFieldIndexer().IndexField(ctx, peer, wireguardRefField, func(obj client.Object) []string {
		// Extract the ConfigMap name from the ConfigDeployment Spec, if one is provided
		peer := obj.(*v1alpha1.WireguardPeer)
		if peer.Spec.WireguardRef == "" {
			return nil
		}
		return []string{peer.Spec.WireguardRef}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.WireguardPeer{}).
		Owns(&v1.Secret{}).
		Watches(
			&source.Kind{Type: &v1alpha1.Wireguard{}},
			handler.EnqueueRequestsFromMapFunc(r.findWireguardRef),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *WireguardPeerReconciler) findWireguardRef(wg client.Object) []reconcile.Request {
	ctx := context.TODO()
	peers := &v1alpha1.WireguardPeerList{}
	opts := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(wireguardRefField, wg.GetName()),
		Namespace:     wg.GetNamespace(),
	}
	if err := r.List(ctx, peers, opts); err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, peer := range peers.Items {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      peer.GetName(),
				Namespace: peer.GetNamespace(),
			},
		}
		requests = append(requests, req)
	}
	return requests
}

func (r *WireguardPeerReconciler) createIfNotExists(
	peer *v1alpha1.WireguardPeer, ctx context.Context, obj client.Object) (bool, error) {

	key := types.NamespacedName{
		Name:      peer.Name,
		Namespace: peer.Namespace,
	}

	err := r.Get(ctx, key, obj)
	if err == nil {
		// resource is already created, nothing to do anymore
		return false, nil
	} else if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}

	if err := r.Create(ctx, obj); err != nil {
		return false, err
	}

	return true, nil
}
