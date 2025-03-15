package controllers

import (
	"context"
	"fmt"

	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
	"github.com/cornbuddy/wireguard-operator/src/private/factory"
)

// WireguardPeerReconciler reconciles a WireguardPeer object
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

func (r *WireguardPeerReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (
	ctrl.Result, error) {

	log := log.FromContext(ctx).WithName("wireguard-peer")

	// WireguardPeer
	peer := &v1alpha1.WireguardPeer{}
	err := r.Get(ctx, req.NamespacedName, peer)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get wireguard peer")
		return ctrl.Result{}, err
	} else if apierrors.IsNotFound(err) {
		log.Info("must have been deleted, reconcilation is finished")
		return ctrl.Result{}, nil
	}
	log.Info("Successfully read peer from cluster, moving on...")

	// Wireguard
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
	requeue := ctrl.Result{Requeue: true}
	if wgPubKey == nil || wgEndpoint == nil {
		log.Info("Corresponding wireguard is not yet reconciled",
			"WireguardRef", peer.Spec.WireguardRef,
			"Wireguard.Status", wireguard.Status)
		fmt.Printf("### %v\n", wireguard.Status)
		return requeue, nil
	}

	fact := factory.Peer{
		Scheme:    r.Scheme,
		Wireguard: *wireguard,
		Peer:      *peer,
	}

	// Secret
	var privateKey, publicKey string
	currentSecret := &v1.Secret{}
	key := types.NamespacedName{
		Name:      peer.GetName(),
		Namespace: peer.GetNamespace(),
	}
	err = r.Get(ctx, key, currentSecret)
	if apierrors.IsNotFound(err) {
		// we need to create a new secret
		key, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			log.Error(err, "Cannot generate keypair")
			return ctrl.Result{}, err
		}

		privateKey = key.String()
		publicKey = key.PublicKey().String()
	} else if err != nil {
		// unexpected error
		log.Error(err, "Cannot fetch corresponding secret from cluster")
		return ctrl.Result{}, err
	} else {
		// secret exists, so let's read keys from it
		privateKey = string(currentSecret.Data["private-key"])
		publicKey = string(currentSecret.Data["public-key"])
	}
	log.Info("Keypair is set")

	desiredSecret, err := fact.Secret(*wgEndpoint, publicKey, privateKey)
	if err != nil {
		log.Error(err, "Cannot generate secret")
		return ctrl.Result{}, err
	}

	if applied, err := apply(ctx, r, desiredSecret); err != nil {
		log.Error(err, "Cannot apply secret")
		return ctrl.Result{}, err
	} else if applied {
		log.Info("Secret applied successfully")
		return requeue, nil
	}
	log.Info("Secret is up to date")

	// Status
	if err := r.Get(ctx, req.NamespacedName, peer); err != nil {
		log.Error(err, "Failed to get wireguard peer")
		return ctrl.Result{}, err
	}

	if peer.Spec.PublicKey == nil {
		peer.Status.PublicKey = &publicKey
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.WireguardPeer{}).
		Owns(&v1.Secret{}).
		Complete(r)
}
