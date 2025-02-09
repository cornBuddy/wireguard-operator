package factory

import (
	"bytes"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
)

type Peer struct {
	*runtime.Scheme
	Peer      v1alpha1.WireguardPeer
	Wireguard v1alpha1.Wireguard
}

func (fact Peer) Secret(endpoint, pubKey, privKey string) (
	*corev1.Secret, error) {

	secret, err := fact.secret(endpoint, pubKey, privKey)
	if err != nil {
		return nil, err
	}

	if err := annotator.SetLastAppliedAnnotation(secret); err != nil {
		return nil, err
	}

	if err := ctrl.SetControllerReference(&fact.Peer, secret, fact.Scheme); err != nil {
		return nil, err
	}

	return secret, nil
}

func (fact Peer) secret(endpoint, publicKey, privateKey string) (
	*corev1.Secret, error) {

	peerPublicKey := *fact.Wireguard.Status.PublicKey
	peer := fact.Peer
	meta := metav1.ObjectMeta{
		Name:      peer.GetName(),
		Namespace: peer.GetNamespace(),
	}
	if peer.Spec.PublicKey != nil {
		return &corev1.Secret{
			ObjectMeta: meta,
			Data: map[string][]byte{
				"public-key": []byte(*peer.Spec.PublicKey),
			},
		}, nil
	}

	tmpl, err := template.New("peer").Parse(peerConfigTemplate)
	if err != nil {
		return nil, err
	}

	var dns string
	wireguard := fact.Wireguard
	if wireguard.Spec.DNS == nil {
		dns = "127.0.0.1"
	} else {
		dns = wireguard.Spec.DNS.Address
	}

	address := fact.Peer.Spec.Address
	spec := peerConfig{
		Address:       address,
		PrivateKey:    privateKey,
		DNS:           dns,
		PeerPublicKey: peerPublicKey,
		Endpoint:      endpoint,
		AllowedIPs:    "0.0.0.0/0",
	}
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, spec); err != nil {
		return nil, err
	}

	config := buf.Bytes()
	secret := &corev1.Secret{
		ObjectMeta: meta,
		Data: map[string][]byte{
			"config":      config,
			"private-key": []byte(privateKey),
			"public-key":  []byte(publicKey),
		},
	}

	return secret, nil
}

const peerConfigTemplate = `[Interface]
Address = {{ .Address }}
PrivateKey = {{ .PrivateKey }}
DNS = {{ .DNS }}

[Peer]
PublicKey = {{ .PeerPublicKey }}
Endpoint = {{ .Endpoint }}
AllowedIPs = {{ .AllowedIPs }}
PersistentKeepalive = 25
`

type peerConfig struct {
	// .spec.Address
	Address v1alpha1.Address
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
