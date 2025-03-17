package factory

import (
	"bytes"
	"fmt"
	"maps"
	"text/template"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
)

const (
	wireguardImage = "linuxserver/wireguard:1.0.20210914"
	wireguardPort  = 51820

	configHashAnnotation  = "vpn.ahova.com/config-hash"
	lastAppliedAnnotation = "vpn.ahova.com/last-applied"

	entrypointSh = `#!/bin/sh
set -e

finish () {
	echo "$(date): Shutting down Wireguard"
	wg-quick down wg0
	exit 0
}

trap finish TERM INT QUIT
echo "$(date): Starting up Wireguard"
wg-quick up wg0

echo "Wireguard started, sleeping..."
sleep infinity`
)

var (
	ErrEndpointNotSet         = fmt.Errorf("public ip not yet set")
	ErrUnsupportedServiceType = fmt.Errorf("unsupported service type")

	annotator = patch.NewAnnotator(lastAppliedAnnotation)
)

type Wireguard struct {
	*runtime.Scheme
	v1alpha1.Wireguard
	Peers v1alpha1.WireguardPeerList
}

// Returns labels for the wireguard resource
func (fact Wireguard) Labels() map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "wireguard-operator",
	}

	maps.Copy(labels, fact.Wireguard.Spec.Labels)
	return labels
}

// Returns desired endpoint address for peer
func (fact Wireguard) ExtractEndpoint(svc corev1.Service) (*string, error) {
	if fact.Wireguard.Spec.EndpointAddress != nil {
		ep := *fact.Wireguard.Spec.EndpointAddress
		return toPtr(fmt.Sprintf("%s:%d", ep, wireguardPort)), nil
	}

	address := ""
	serviceType := fact.Wireguard.Spec.ServiceType
	switch serviceType {
	case corev1.ServiceTypeClusterIP:
		address = svc.Spec.ClusterIP
	case corev1.ServiceTypeLoadBalancer:
		address = fact.extractEndpointFromLoadBalancer(svc)
	default:
		return nil, ErrUnsupportedServiceType
	}

	if address == "" {
		return nil, ErrEndpointNotSet
	}

	result := fmt.Sprintf("%s:%d", address, wireguardPort)
	return &result, nil
}

// Returns empty string when public address of LB is not set
func (fact Wireguard) extractEndpointFromLoadBalancer(svc corev1.Service) string {
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return ""
	}

	ingress := svc.Status.LoadBalancer.Ingress[0]
	if ingress.IP != "" {
		return ingress.IP
	} else if ingress.Hostname != "" {
		return ingress.Hostname
	} else {
		return ""
	}
}

func (fact Wireguard) ConfigMap() (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fact.Wireguard.GetName(),
			Namespace: fact.Wireguard.GetNamespace(),
			Labels:    fact.Labels(),
		},
		Data: map[string]string{
			"entrypoint.sh": entrypointSh,
		},
	}

	result, err := fact.decorate(cm)
	if err != nil {
		return nil, err
	}

	return result.(*corev1.ConfigMap), nil
}

func (fact Wireguard) Service() (*corev1.Service, error) {
	wg := fact.Wireguard

	var externalTrafficPolicy corev1.ServiceExternalTrafficPolicy
	if wg.Spec.ServiceType == corev1.ServiceTypeLoadBalancer {
		externalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
	} else {
		externalTrafficPolicy = ""
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        wg.GetName(),
			Namespace:   wg.GetNamespace(),
			Labels:      fact.Labels(),
			Annotations: wg.Spec.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     wg.Spec.ServiceType,
			Selector: fact.Labels(),
			Ports: []corev1.ServicePort{{
				Name:     "wireguard",
				Protocol: "UDP",
				Port:     wireguardPort,
			}},
			ExternalTrafficPolicy: externalTrafficPolicy,
		},
	}

	result, err := fact.decorate(svc)
	if err != nil {
		return nil, err
	}

	return result.(*corev1.Service), nil
}

// Returns desired secret for the current wireguard instance
func (fact Wireguard) Secret(pubKey, privKey string) (*corev1.Secret, error) {
	tmpl, err := template.New("config").Parse(serverConfigTemplate)
	if err != nil {
		return nil, err
	}

	var wireguardPeers []serverPeer
	for _, peer := range fact.Peers.Items {
		// somehow expected: peer crd is created, but not yet reconciled
		if peer.Status.PublicKey == nil {
			continue
		}

		allowedIPs := peer.Spec.Address
		wireguardPeers = append(wireguardPeers, serverPeer{
			AllowedIPs:   allowedIPs,
			FriendlyName: peer.GetName(),
			PublicKey:    *peer.Status.PublicKey,
		})
	}
	spec := serverConfig{
		Address:           fact.Wireguard.Spec.Address,
		PrivateKey:        string(privKey),
		ListenPort:        wireguardPort,
		DropConnectionsTo: fact.Wireguard.Spec.DropConnectionsTo,
		Peers:             wireguardPeers,
	}
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, spec); err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fact.Wireguard.Name,
			Namespace: fact.Wireguard.Namespace,
			Labels:    fact.Labels(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"config":      buf.Bytes(),
			"public-key":  []byte(pubKey),
			"private-key": []byte(privKey),
		},
	}

	result, err := fact.decorate(secret)
	if err != nil {
		return nil, err
	}

	return result.(*corev1.Secret), nil
}

func (fact Wireguard) Deployment(configHash string) (*appsv1.Deployment, error) {
	deploy := fact.deployment(configHash)
	result, err := fact.decorate(&deploy)
	if err != nil {
		return nil, err
	}

	return result.(*appsv1.Deployment), nil
}

// Returns desired deployment for the current wireguard instance
func (fact Wireguard) deployment(configHash string) appsv1.Deployment {
	wireguard := fact.Wireguard
	volumes := []corev1.Volume{{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: wireguard.Name,
				Items: []corev1.KeyToPath{{
					Key:  "config",
					Path: "wg0.conf",
				}},
			},
		},
	}, {
		Name: "entrypoint",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: wireguard.Name,
				},
				Items: []corev1.KeyToPath{{
					Key:  "entrypoint.sh",
					Path: "entrypoint.sh",
					Mode: toPtr[int32](0755),
				}},
			},
		},
	}}
	mounts := []corev1.VolumeMount{{
		Name:      "config",
		MountPath: "/etc/wireguard",
	}, {
		Name:      "entrypoint",
		MountPath: "/opt/bin",
	}}
	wireguardContainer := corev1.Container{
		Image:           wireguardImage,
		Name:            "wireguard",
		Command:         []string{"/opt/bin/entrypoint.sh"},
		ImagePullPolicy: corev1.PullIfNotPresent,
		VolumeMounts:    mounts,
		SecurityContext: &corev1.SecurityContext{
			Privileged: toPtr(true),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN",
					"SYS_MODULE",
				},
			},
		},
		Ports: []corev1.ContainerPort{{
			ContainerPort: wireguardPort,
			Name:          "wireguard",
			Protocol:      "UDP",
		}},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"/bin/sh",
						"-c",
						"ip link show wg0 up",
					},
				},
			},
			FailureThreshold:    2,
			SuccessThreshold:    1,
			InitialDelaySeconds: 5,
			TimeoutSeconds:      1,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Host:   "google.com",
					Scheme: "HTTPS",
					Path:   "/",
					Port:   intstr.IntOrString{IntVal: 443},
					HTTPHeaders: []corev1.HTTPHeader{{
						Name:  "Host",
						Value: "www.google.com",
					}},
				},
			},
			FailureThreshold:    2,
			SuccessThreshold:    3,
			InitialDelaySeconds: 10,
			TimeoutSeconds:      10,
			PeriodSeconds:       10,
		},
	}
	containers := append(
		[]corev1.Container{wireguardContainer},
		wireguard.Spec.Sidecars...,
	)
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: fact.Labels(),
			Annotations: map[string]string{
				configHashAnnotation: configHash,
			},
		},
		Spec: corev1.PodSpec{
			Affinity:   wireguard.Spec.Affinity,
			Containers: containers,
			SecurityContext: &corev1.PodSecurityContext{
				Sysctls: []corev1.Sysctl{{
					Name:  "net.ipv4.ip_forward",
					Value: "1",
				}, {
					Name:  "net.ipv4.conf.all.src_valid_mark",
					Value: "1",
				}, {
					Name:  "net.ipv4.conf.all.rp_filter",
					Value: "0",
				}, {
					Name:  "net.ipv4.conf.all.route_localnet",
					Value: "1",
				}},
			},
			Volumes: volumes,
		},
	}

	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wireguard.Name,
			Namespace: wireguard.Namespace,
			Labels:    fact.Labels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &wireguard.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: fact.Labels(),
			},
			Template: podTemplate,
		},
	}
}

func (fact Wireguard) decorate(obj client.Object) (client.Object, error) {
	wg := &fact.Wireguard
	scheme := fact.Scheme
	if err := ctrl.SetControllerReference(wg, obj, scheme); err != nil {
		return nil, err
	}

	if err := annotator.SetLastAppliedAnnotation(obj); err != nil {
		return nil, err
	}

	return obj, nil
}

func toPtr[V any](o V) *V { return &o }

type serverPeer struct {
	AllowedIPs   v1alpha1.Address
	FriendlyName string
	PublicKey    string
}

type serverConfig struct {
	Address           v1alpha1.Address
	PrivateKey        string
	ListenPort        int32
	DropConnectionsTo []string
	Peers             []serverPeer
}

const serverConfigTemplate = `[Interface]
Address = {{ .Address }}
PrivateKey = {{ .PrivateKey }}
ListenPort = {{ .ListenPort }}
{{- range .DropConnectionsTo }}
PostUp = iptables --insert FORWARD --source {{ $.Address }} --destination {{ . }} --jump DROP
{{- end }}
PostUp = iptables --append FORWARD --in-interface %i --jump ACCEPT
PostUp = iptables --append FORWARD --out-interface %i --jump ACCEPT
PostUp = iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
SaveConfig = false
{{ range .Peers }}
[Peer]
# friendly_name = {{ .FriendlyName }}
PublicKey = {{ .PublicKey }}
AllowedIPs = {{ .AllowedIPs }}
{{ end }}`
