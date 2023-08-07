package controllers

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/dsl"
)

// WireguardPeerReconciler reconciles a Wireguard object
type WireguardPeerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguardpeers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *WireguardPeerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).
		WithName("wireguard-peer").
		WithValues("Name", req.Name, "Namespace", req.Namespace)

	peer := &vpnv1alpha1.WireguardPeer{}
	err := r.Get(ctx, req.NamespacedName, peer)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get wireguard peer")
		return ctrl.Result{}, err
	} else if apierrors.IsNotFound(err) {
		log.Info("wireguardpeer resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}
	log.Info("Successfully read peer from cluster, moving on...")

	wireguard := &vpnv1alpha1.Wireguard{}
	wgKey := types.NamespacedName{
		Name:      peer.Spec.WireguardRef,
		Namespace: peer.GetNamespace(),
	}
	if err := r.Get(ctx, wgKey, wireguard); err != nil {
		log.Error(err, "Cannot retrieve parent wireguard resource")
		return ctrl.Result{}, nil
	}
	log.Info("Retrieved parent wireguard resource, moving on...")

	wgService := &v1.Service{}
	if err := r.Get(ctx, wgKey, wgService); err != nil {
		log.Info("Cannot fetch wireguard service, skipping the loop...")
		return ctrl.Result{Requeue: true}, nil
	}
	log.Info("Retrieved wireguard service, moving on...")

	peerDsl := dsl.Peer{
		PeerSpec:      peer.Spec,
		WireguardSpec: wireguard.Spec,
	}
	endpoint, err := peerDsl.ExtractEndpoint(*wgService)
	if err == dsl.ErrPublicIpNotSet {
		log.Info("Public ip not yet set, skipping the loop...")
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Cannot extract endpoint")
		return ctrl.Result{}, err
	}
	log.Info("Peer endpoint is calculated, moving on...", "Endpoint", endpoint)

	secret, err := r.getSecret(peer, wireguard, endpoint)
	if err != nil {
		log.Error(err, "Cannot generate secret")
		return ctrl.Result{}, err
	}
	if created, err := r.createIfNotExists(peer, ctx, secret); err != nil {
		log.Error(err, "Cannot create secret")
		return ctrl.Result{}, err
	} else if created {
		log.Info("Secret created successfully")
		return ctrl.Result{Requeue: true}, nil
	}
	log.Info("Secret is created already, moving on...")

	return ctrl.Result{}, nil
}

func (r *WireguardPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpnv1alpha1.WireguardPeer{}).
		Owns(&v1.Secret{}).
		Complete(r)
}

type peerConfig struct {
	// .spec.Address
	Address string
	// private key of the peer
	PrivateKey string
	// wireguard.spec.DNS.address
	DNS string
	// public key of the parent wireguard resource
	PeerPublicKey string
	// public endpoint of the wireguard service
	Endpoint   string
	AllowedIPs string
}

const peerConfigTemplate = `[Interface]
Address = {{ .Address }}/32
PrivateKey = {{ .PrivateKey }}
DNS = {{ .DNS }}

[Peer]
PublicKey = {{ .PeerPublicKey }}
Endpoint = {{ .Endpoint }}
AllowedIPs = {{ .AllowedIPs }}`

func (r *WireguardPeerReconciler) getSecret(
	peer *vpnv1alpha1.WireguardPeer, wireguard *vpnv1alpha1.Wireguard, endpoint string) (*v1.Secret, error) {

	var data map[string][]byte
	if peer.Spec.PublicKey == nil {
		tmpl, err := template.New("peer").Parse(peerConfigTemplate)
		if err != nil {
			return nil, err
		}

		key, err := wgtypes.GenerateKey()
		if err != nil {
			return nil, err
		}

		privateKey := []byte(key.String())
		publicKey := []byte(key.PublicKey().String())

		wgKey := types.NamespacedName{
			Name:      wireguard.GetName(),
			Namespace: wireguard.GetNamespace(),
		}
		wgSecret := &v1.Secret{}
		if err := r.Get(context.TODO(), wgKey, wgSecret); err != nil {
			return nil, err
		}

		wgSvc := &v1.Service{}
		if err := r.Get(context.TODO(), wgKey, wgSvc); err != nil {
			return nil, err
		}

		spec := peerConfig{
			Address:       peer.Spec.Address,
			PrivateKey:    key.String(),
			DNS:           wireguard.Spec.DNS.Address,
			PeerPublicKey: string(wgSecret.Data["public-key"]),
			Endpoint:      fmt.Sprintf("%s:%d", endpoint, wireguardPort),
			AllowedIPs:    "0.0.0.0/0, ::/0",
		}
		buf := new(bytes.Buffer)
		if err := tmpl.Execute(buf, spec); err != nil {
			return nil, err
		}

		data = map[string][]byte{
			"config":      buf.Bytes(),
			"public-key":  publicKey,
			"private-key": privateKey,
		}
	} else {
		data = map[string][]byte{
			"public-key": []byte(*peer.Spec.PublicKey),
		}
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peer.GetName(),
			Namespace: peer.GetNamespace(),
		},
		Data: data,
	}
	return secret, nil
}

func (r *WireguardPeerReconciler) createIfNotExists(
	peer *vpnv1alpha1.WireguardPeer, ctx context.Context, obj client.Object) (bool, error) {

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

	if err := ctrl.SetControllerReference(peer, obj, r.Scheme); err != nil {
		return false, err
	}

	if err := r.Create(ctx, obj); err != nil {
		return false, err
	}

	return true, nil
}
