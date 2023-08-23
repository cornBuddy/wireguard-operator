package controllers

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/factory"
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

func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).
		WithName("wireguard").
		WithValues("Name", req.Name, "Namespace", req.Namespace)

	wireguard := &v1alpha1.Wireguard{}
	err := r.Get(ctx, req.NamespacedName, wireguard)
	if err != nil && !apierrors.IsNotFound(err) {
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get wireguard")
		return ctrl.Result{}, err
	} else if apierrors.IsNotFound(err) {
		// If the custom resource is not found then, it usually means
		// that it was deleted or not created. In this way, we will stop
		// the reconciliation
		log.Info("wireguard resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}
	log.Info("Successfully read wireguard from cluster")

	peers, err := r.getPeers(ctx, wireguard)
	if err != nil {
		log.Error(err, "Cannot list related peers")
		return ctrl.Result{}, err
	}
	log.Info("Peers list is fetched")

	fact := factory.Wireguard{
		Scheme:    r.Scheme,
		Wireguard: *wireguard,
		Peers:     peers,
	}
	service, err := fact.Service()
	if err != nil {
		log.Error(err, "Cannot generate service")
		return ctrl.Result{}, err
	}

	if applied, err := r.apply(ctx, service); err != nil {
		log.Error(err, "Cannot apply service")
		return ctrl.Result{}, err
	} else if applied {
		log.Info("Service applied successfully")
		return ctrl.Result{}, nil
	}
	log.Info("Service is applied already")

	cm, err := fact.ConfigMap()
	if err != nil {
		log.Error(err, "Cannot generate configmap")
		return ctrl.Result{}, err
	}

	if applied, err := r.apply(ctx, cm); err != nil {
		log.Error(err, "Cannot apply configmap")
		return ctrl.Result{}, err
	} else if applied {
		log.Info("Configmap applied successfully")
		return ctrl.Result{}, nil
	}
	log.Info("Config map is applied already")

	var privateKey, publicKey string
	currentSecret := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      wireguard.GetName(),
		Namespace: wireguard.GetNamespace(),
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
		// assuming that secret exists
		privateKey = string(currentSecret.Data["private-key"])
		publicKey = string(currentSecret.Data["public-key"])
	}
	log.Info("Keypair is set")

	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		log.Error(err, "Cannot read service from the cluster")
		return ctrl.Result{}, err
	}
	log.Info("Successfully read service from cluster")

	ep := fact.ExtractEndpoint(*service)
	if ep == nil {
		log.Info("Cannot extract endpoint from service")
		return ctrl.Result{}, nil
	}

	desiredSecret, err := fact.Secret(publicKey, privateKey, *ep)
	if err != nil {
		log.Error(err, "Cannot generate secret")
		return ctrl.Result{}, err
	}

	if applied, err := r.apply(ctx, desiredSecret); err != nil {
		log.Error(err, "Cannot apply secret")
		return ctrl.Result{}, err
	} else if applied {
		log.Info("Secret applied successfully")
		return ctrl.Result{}, nil
	}
	log.Info("Secret is applied")

	configHash := makeHash(desiredSecret.Data["config"])
	deploy, err := fact.Deployment(configHash)
	if err != nil {
		log.Error(err, "Cannot generate deployment")
		return ctrl.Result{}, err
	}

	if created, err := r.apply(ctx, deploy); err != nil {
		log.Error(err, "Cannot apply deployment")
		return ctrl.Result{}, err
	} else if created {
		log.Info("Deployment applied successfully")
		return ctrl.Result{}, nil
	}
	log.Info("Deployment is applied already")

	wireguard.Status = v1alpha1.WireguardStatus{
		Endpoint:  ep,
		PublicKey: &publicKey,
	}
	if err := r.Status().Update(ctx, wireguard); err != nil {
		log.Error(err, "Cannot update status")
		return ctrl.Result{}, err
	}
	log.Info("Status is updated")

	return ctrl.Result{}, nil
}

func (r *WireguardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Wireguard{}).
		Owns(&v1alpha1.WireguardPeer{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *WireguardReconciler) getPeers(
	ctx context.Context, wg *v1alpha1.Wireguard) (v1alpha1.WireguardPeerList, error) {

	var allPeers v1alpha1.WireguardPeerList
	err := r.List(ctx, &allPeers)
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

// creates or updates resource. returns true when created or updated
func (r *WireguardReconciler) apply(
	ctx context.Context, desired client.Object) (bool, error) {
	var current client.Object
	switch v := desired.(type) {
	case *corev1.Service:
		current = &corev1.Service{}
	case *corev1.ConfigMap:
		current = &corev1.ConfigMap{}
	case *corev1.Secret:
		current = &corev1.Secret{}
	case *appsv1.Deployment:
		current = &appsv1.Deployment{}
	case nil:
		return false, fmt.Errorf("desired cannot be nil")
	default:
		return false, fmt.Errorf("unsupported type %s for desired", v)
	}

	key := types.NamespacedName{
		Name:      desired.GetName(),
		Namespace: desired.GetNamespace(),
	}
	err := r.Get(ctx, key, current)
	unexpectedError := err != nil && !apierrors.IsNotFound(err)
	if unexpectedError {
		return false, err
	}

	objectNotExists := apierrors.IsNotFound(err)
	if objectNotExists {
		if err := r.Create(ctx, desired); err != nil {
			return false, err
		}

		// resource is created, get back to reconcilation
		return true, nil
	}

	patchMaker := patch.DefaultPatchMaker
	opts := []patch.CalculateOption{
		patch.IgnoreField("metadata"),
	}
	patchResult, err := patchMaker.Calculate(current, desired, opts...)
	if err != nil {
		return false, err
	}

	// nothing to update, get back to reconcilation
	if patchResult.IsEmpty() {
		return false, nil
	}

	if err := r.Update(ctx, desired); err != nil {
		return false, err
	}

	// resource is updated, get back to reconcilation
	return true, nil
}

func makeHash(data []byte) string {
	hash := sha1.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}
