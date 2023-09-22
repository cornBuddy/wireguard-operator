package factory

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/cisco-open/k8s-objectmatcher/patch"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

const (
	dnsImage       = "docker.io/klutchell/unbound:v1.17.1"
	wireguardImage = "linuxserver/wireguard:1.0.20210914"
	wireguardPort  = 51820

	configHashAnnotation  = "vpn.ahova.com/config-hash"
	lastAppliedAnnotation = "vpn.ahova.com/last-applied"
)

var (
	ErrPublicIpNotYetSet = fmt.Errorf("public ip not yet set")

	annotator = patch.NewAnnotator(lastAppliedAnnotation)
)

type Wireguard struct {
	*runtime.Scheme
	v1alpha1.Wireguard
	Peers v1alpha1.WireguardPeerList
}

// Returns desired endpoint address for peer
func (fact Wireguard) ExtractEndpoint(svc corev1.Service) (*string, error) {
	if fact.Wireguard.Spec.EndpointAddress != nil {
		ep := *fact.Wireguard.Spec.EndpointAddress
		return toPtr(fmt.Sprintf("%s:%d", ep, wireguardPort)), nil
	}

	serviceType := fact.Wireguard.Spec.ServiceType
	address := ""
	switch serviceType {
	case corev1.ServiceTypeClusterIP:
		address = svc.Spec.ClusterIP
	case corev1.ServiceTypeLoadBalancer:
		address = fact.extractEndpointFromLoadBalancer(svc)
	default:
		return nil, fmt.Errorf("unsupported service type")
	}

	publicIpNotYetSet := address == "" &&
		serviceType == corev1.ServiceTypeLoadBalancer
	if publicIpNotYetSet {
		return nil, ErrPublicIpNotYetSet
	}

	if address == "" {
		return nil, fmt.Errorf("cannot extract endpoint from service")
	}

	result := fmt.Sprintf("%s:%d", address, wireguardPort)
	return &result, nil
}

// Returns empty string when public address of LB is not set
func (fact Wireguard) extractEndpointFromLoadBalancer(svc corev1.Service) string {
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return svc.Spec.ClusterIP
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
	tmpl, err := template.New("unbound").Parse(unboundConfTmpl)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, fact.Wireguard.Spec); err != nil {
		return nil, err
	}

	unboundConf := buf.String()
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fact.Wireguard.GetName(),
			Namespace: fact.Wireguard.GetNamespace(),
		},
		Data: map[string]string{
			"unbound.conf": unboundConf,
		},
	}

	if err := annotator.SetLastAppliedAnnotation(configMap); err != nil {
		return nil, err
	}

	if err := ctrl.SetControllerReference(&fact.Wireguard, configMap, fact.Scheme); err != nil {
		return nil, err
	}

	return configMap, nil
}

func (fact Wireguard) Service() (*corev1.Service, error) {
	wg := fact.Wireguard
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        wg.GetName(),
			Namespace:   wg.GetNamespace(),
			Annotations: wg.Spec.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     wg.Spec.ServiceType,
			Selector: getLabels(wg.Name),
			Ports: []corev1.ServicePort{{
				Name:     "wireguard",
				Protocol: "UDP",
				Port:     wireguardPort,
			}},
		},
	}

	if err := annotator.SetLastAppliedAnnotation(service); err != nil {
		return nil, err
	}

	if err := ctrl.SetControllerReference(&fact.Wireguard, service, fact.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

// Returns desired secret for the current wireguard instance
func (fact Wireguard) Secret(pubKey, privKey, peerEndpoint string) (*corev1.Secret, error) {
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

		wireguardPeers = append(wireguardPeers, serverPeer{
			PublicKey: *peer.Status.PublicKey,
			Address:   peer.Spec.Address,
			Endpoint:  peerEndpoint,
		})
	}
	spec := serverConfig{
		Address:           fact.Wireguard.Spec.Network,
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
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"config":      buf.Bytes(),
			"public-key":  []byte(pubKey),
			"private-key": []byte(privKey),
		},
	}

	if err := annotator.SetLastAppliedAnnotation(secret); err != nil {
		return nil, err
	}

	if err := ctrl.SetControllerReference(&fact.Wireguard, secret, fact.Scheme); err != nil {
		return nil, err
	}

	return secret, nil
}

func (fact Wireguard) Deployment(configHash string) (*appsv1.Deployment, error) {
	deploy := fact.deployment(configHash)
	if err := annotator.SetLastAppliedAnnotation(&deploy); err != nil {
		return nil, err
	}

	if err := ctrl.SetControllerReference(&fact.Wireguard, &deploy, fact.Scheme); err != nil {
		return nil, err
	}

	return &deploy, nil
}

// Returns desired deployment for the current wireguard instance
func (fact Wireguard) deployment(configHash string) appsv1.Deployment {
	wireguard := fact.Wireguard
	volumes, mounts := getVolumes(wireguard)
	wireguardContainer := corev1.Container{
		Image:           wireguardImage,
		Name:            "wireguard",
		ImagePullPolicy: corev1.PullIfNotPresent,
		VolumeMounts:    mounts.wireguard,
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
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getLabels(wireguard.Name),
			Annotations: map[string]string{
				configHashAnnotation: configHash,
			},
		},
		Spec: corev1.PodSpec{
			Affinity: wireguard.Spec.Affinity,
			Containers: []corev1.Container{
				wireguardContainer,
			},
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
	unboundContainer := corev1.Container{
		Image:           dnsImage,
		Name:            "unbound",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"unbound"},
		VolumeMounts:    mounts.unbound,
		Args: []string{
			"-d",
			"-c",
			"/etc/unbound/unbound.conf",
		},
	}

	if wireguard.Spec.DNS == nil {
		podTemplate.Spec.DNSPolicy = corev1.DNSNone
		podTemplate.Spec.DNSConfig = &corev1.PodDNSConfig{
			Nameservers: []string{"127.0.0.1"},
		}
		podTemplate.Spec.Containers = append(podTemplate.Spec.Containers, unboundContainer)

		// returning early to skip further not nil check
		return appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wireguard.Name,
				Namespace: wireguard.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &wireguard.Spec.Replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: getLabels(wireguard.Name),
				},
				Template: podTemplate,
			},
		}
	}

	if wireguard.Spec.DNS.DeployServer {
		podTemplate.Spec.DNSPolicy = corev1.DNSNone
		podTemplate.Spec.DNSConfig = &corev1.PodDNSConfig{
			Nameservers: []string{"127.0.0.1"},
		}
		podTemplate.Spec.Containers = append(podTemplate.Spec.Containers, unboundContainer)
	} else {
		podTemplate.Spec.DNSPolicy = corev1.DNSClusterFirst
		podTemplate.Spec.DNSConfig = nil
	}

	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wireguard.Name,
			Namespace: wireguard.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &wireguard.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(wireguard.Name),
			},
			Template: podTemplate,
		},
	}
}

type containerMounts struct {
	unbound   []corev1.VolumeMount
	wireguard []corev1.VolumeMount
}

func getVolumes(wireguard v1alpha1.Wireguard) ([]corev1.Volume, containerMounts) {
	volumes := []corev1.Volume{{
		Name: "wireguard-config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: wireguard.Name,
				Items: []corev1.KeyToPath{{
					Key:  "config",
					Path: "wg0.conf",
				}},
			},
		},
	}}
	mounts := containerMounts{
		unbound: []corev1.VolumeMount{},
		wireguard: []corev1.VolumeMount{{
			Name:      "wireguard-config",
			ReadOnly:  true,
			MountPath: "/config",
		}},
	}

	if wireguard.Spec.DNS == nil {
		volumes = append(volumes, corev1.Volume{
			Name: "unbound-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: wireguard.Name,
					},
					Items: []corev1.KeyToPath{{
						Key:  "unbound.conf",
						Path: "unbound.conf",
					}},
				},
			},
		})
		mounts.unbound = append(mounts.unbound, corev1.VolumeMount{
			Name:      "unbound-config",
			ReadOnly:  true,
			MountPath: "/etc/unbound",
		})

		// returning early to skip further not nil check
		return volumes, mounts
	}

	if wireguard.Spec.DNS.DeployServer {
		volumes = append(volumes, corev1.Volume{
			Name: "unbound-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: wireguard.Name,
					},
					Items: []corev1.KeyToPath{{
						Key:  "unbound.conf",
						Path: "unbound.conf",
					}},
				},
			},
		})
		mounts.unbound = append(mounts.unbound, corev1.VolumeMount{
			Name:      "unbound-config",
			ReadOnly:  true,
			MountPath: "/etc/unbound",
		})
	}
	return volumes, mounts
}

func getLabels(name string) map[string]string {
	imageTag := strings.Split(wireguardImage, ":")[1]
	return map[string]string{
		"app.kubernetes.io/name":       "Wireguard",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "wireguard-operator",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

func toPtr[V any](o V) *V { return &o }

type serverPeer struct {
	PublicKey string
	Address   string
	Endpoint  string
}

type serverConfig struct {
	Address           string
	PrivateKey        string
	ListenPort        int32
	DropConnectionsTo []string
	Peers             []serverPeer
}

// represents go-template for the server config
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
PublicKey = {{ .PublicKey }}
AllowedIPs = {{ .Address }}/32
Endpoint = {{ .Endpoint }}
PersistentKeepalive = 25
{{ end }}`

// template of the unbound config
const unboundConfTmpl = `
remote-control:
	control-enable: yes
	control-interface: 127.0.0.1
	control-use-cert: no
server:
	num-threads: 1
	verbosity: 1
	interface: 0.0.0.0
	max-udp-size: 3072
	access-control: 0.0.0.0/0 refuse
	access-control: 127.0.0.1 allow
	access-control: {{ .Network }} allow
	private-address: {{ .Network }}
	hide-identity: yes
	hide-version: yes
	harden-glue: yes
	harden-dnssec-stripped: yes
	harden-referral-path: yes
	unwanted-reply-threshold: 10000000
	val-log-level: 1
	cache-min-ttl: 1800
	cache-max-ttl: 14400
	prefetch: yes
	prefetch-key: yes`
