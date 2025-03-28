package controllers

import (
	"context"

	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
	"github.com/cornbuddy/wireguard-operator/src/private/factory"
)

// WireguardReconciler reconciles a Wireguard object
type WireguardReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguards/status,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguardpeers,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguardpeers/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;list;watch;create;update;patch;delete

func (r *WireguardReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (
	ctrl.Result, error) {

	empty := ctrl.Result{}
	requeue := ctrl.Result{Requeue: true}
	log := log.FromContext(ctx).WithName("wireguard")

	// Wireguard
	wireguard, err := r.getWireguard(ctx, req.NamespacedName)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get wireguard")
		return empty, err
	} else if apierrors.IsNotFound(err) {
		log.Info("Must have been deleted, reconcilation is finished")
		return empty, nil
	}
	log.Info("Successfully read wireguard from cluster")

	// WireguardPeers
	peers, err := r.getPeers(ctx, wireguard)
	if err != nil {
		log.Error(err, "Cannot list related peers")
		return empty, err
	}
	log.Info("Peers list is fetched", "peers", peers.Items)

	fact := factory.Wireguard{
		Scheme:    r.Scheme,
		Wireguard: *wireguard,
		Peers:     peers,
	}

	// Service
	service, err := fact.Service()
	if err != nil {
		log.Error(err, "Cannot generate service")
		return empty, err
	}

	if applied, err := apply(ctx, r, service); err != nil {
		log.Error(err, "Cannot apply service")
		return empty, err
	} else if applied {
		log.Info("Service applied successfully")
		return requeue, nil
	}
	log.Info("Service is up to date")

	// ConfigMap
	cm, err := fact.ConfigMap()
	if err != nil {
		log.Error(err, "Cannot generate configmap")
		return empty, err
	}

	if applied, err := apply(ctx, r, cm); err != nil {
		log.Error(err, "Cannot apply configmap")
		return empty, err
	} else if applied {
		log.Info("Configmap applied successfully")
		return requeue, nil
	}
	log.Info("Config map is up to date")

	// Secret
	var privateKey, publicKey string
	key := types.NamespacedName{
		Name:      wireguard.GetName(),
		Namespace: wireguard.GetNamespace(),
	}
	currentSecret := &corev1.Secret{}
	err = r.Get(ctx, key, currentSecret)
	if apierrors.IsNotFound(err) {
		// we need to create a new secret
		key, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			log.Error(err, "Cannot generate keypair")
			return empty, err
		}

		privateKey = key.String()
		publicKey = key.PublicKey().String()
	} else if err != nil {
		// unexpected error
		log.Error(err, "Cannot fetch corresponding secret from cluster")
		return empty, err
	} else {
		// secret exists, so let's read keys from it
		privateKey = string(currentSecret.Data["private-key"])
		publicKey = string(currentSecret.Data["public-key"])
	}
	log.Info("Keypair is set")

	desiredSecret, err := fact.Secret(publicKey, privateKey)
	if err != nil {
		log.Error(err, "Cannot generate secret")
		return empty, err
	}

	if applied, err := apply(ctx, r, desiredSecret); err != nil {
		log.Error(err, "Cannot apply secret")
		return empty, err
	} else if applied {
		log.Info("Secret applied successfully")
		return requeue, nil
	}
	log.Info("Secret is up to date")

	// Deployment
	configHash := makeHash(desiredSecret.Data["config"])
	deploy, err := fact.Deployment(configHash)
	if err != nil {
		log.Error(err, "Cannot generate deployment")
		return empty, err
	}

	if applied, err := apply(ctx, r, deploy); err != nil {
		log.Error(err, "Cannot apply deployment")
		return empty, err
	} else if applied {
		log.Info("Deployment applied successfully")
		return requeue, nil
	}
	log.Info("Deployment is up to date")

	// FIXME: when reconcilation is triggered by peer, code below fails,
	// because `key` contains peer name, whilst it must contain wireguard
	// name

	// Status
	if err := r.Get(ctx, key, service); err != nil {
		log.Error(err, "Cannot read service from the cluster")
		return empty, err
	}
	log.Info("Successfully read service from cluster")

	ep, err := fact.ExtractEndpoint(*service)
	if err == factory.ErrEndpointNotSet {
		log.Info("Public ip not yet set, somehow expected")
		return requeue, nil
	} else if err != nil {
		log.Error(err, "Cannot extract endpoint from service")
		return empty, err
	}

	if err := r.Get(ctx, key, wireguard); err != nil {
		log.Error(err, "Failed to refetch wireguard")
		return empty, err
	}

	wireguard.Status = v1alpha1.WireguardStatus{
		Endpoint:  ep,
		PublicKey: &publicKey,
	}
	if err := r.Status().Update(ctx, wireguard); err != nil {
		log.Error(err, "Cannot update status")
		return empty, err
	}
	log.Info("Status is updated, reconcilation is finished")

	return empty, nil
}

func (r *WireguardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	handlers := &handler.EnqueueRequestForObject{}
	predicates := builder.WithPredicates(
		predicate.ResourceVersionChangedPredicate{},
	)
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Wireguard{}).
		Watches(&v1alpha1.WireguardPeer{}, handlers, predicates).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *WireguardReconciler) getWireguard(
	ctx context.Context, key types.NamespacedName) (
	*v1alpha1.Wireguard, error) {

	wireguard := &v1alpha1.Wireguard{}
	err := r.Get(ctx, key, wireguard)
	if err != nil && !apierrors.IsNotFound(err) {
		// unexpected error
		return nil, err
	} else if err == nil {
		// wireguard found, can return it
		return wireguard, nil
	}

	// this is somehow expected. either reconcilation was triggered
	// by peer (see SetupWithManager), or wireguard resource was
	// deleted. checking if it was triggered by peer
	peer := &v1alpha1.WireguardPeer{}
	if err := r.Get(ctx, key, peer); err != nil {
		return nil, err
	}

	// peer was found, but we still need to fetch wireguard resource
	wgKey := types.NamespacedName{
		Name:      peer.Spec.WireguardRef,
		Namespace: peer.GetNamespace(),
	}
	if err := r.Get(ctx, wgKey, wireguard); err != nil {
		return nil, err
	}

	// successfully found wireguard, can finally return it
	return wireguard, nil
}

func (r *WireguardReconciler) getPeers(
	ctx context.Context, wg client.Object) (v1alpha1.WireguardPeerList, error) {

	var allPeers v1alpha1.WireguardPeerList
	opts := &client.ListOptions{Namespace: wg.GetNamespace()}
	err := r.List(ctx, &allPeers, opts)
	if err != nil {
		return v1alpha1.WireguardPeerList{}, err
	}

	peers := v1alpha1.WireguardPeerList{
		Items: []v1alpha1.WireguardPeer{},
	}
	for _, peer := range allPeers.Items {
		if peer.Spec.WireguardRef == wg.GetName() {
			peers.Items = append(peers.Items, peer)
		}
	}

	return peers, nil
}
