package controllers

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

const (
	wireguardPort = 51820
)

// WireguardReconciler reconciles a Wireguard object
type WireguardReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguardpeers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *WireguardReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := log.FromContext(ctx).
		WithName("wireguard").
		WithValues("Name", req.Name, "Namespace", req.Namespace)

	wireguard := &vpnv1alpha1.Wireguard{}
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
	log.Info("Successfully read wireguard from cluster, moving on...")

	svc := r.getService(wireguard)
	if createdOrUpdated, err := r.createOrUpdateService(wireguard, ctx, svc); err != nil {
		log.Error(err, "Cannot create service")
		return ctrl.Result{}, err
	} else if createdOrUpdated {
		log.Info("Service reconciled successfully")
		return ctrl.Result{Requeue: true}, nil
	}
	log.Info("Refetching service from the cluster...")
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		log.Error(err, "Cannot refetch service from the cluster")
		return ctrl.Result{}, err
	}
	log.Info("Service is up to date already, moving on...")

	cm, err := r.getConfigMap(wireguard)
	if err != nil {
		log.Error(err, "Cannot generate configmap")
		return ctrl.Result{}, err
	}
	if created, err := r.createIfNotExists(wireguard, ctx, cm); err != nil {
		log.Error(err, "Cannot create configmap")
		return ctrl.Result{}, err
	} else if created {
		log.Info("Configmap created successfully")
		return ctrl.Result{Requeue: true}, nil
	}
	log.Info("Configmap is created already, moving on...")

	peers, err := r.getPeers(wireguard, ctx)
	if err != nil {
		log.Error(err, "Cannot generate related peers")
		return ctrl.Result{}, err
	}
	secret, err := r.getSecret(wireguard, peers, svc.Spec.ClusterIP)
	if err != nil {
		log.Error(err, "Cannot generate secret")
		return ctrl.Result{}, err
	}
	if createdOrUpdated, err := r.createOrUpdateSecret(wireguard, ctx, secret); err != nil {
		log.Error(err, "Cannot create secret")
		return ctrl.Result{}, err
	} else if createdOrUpdated {
		log.Info("Secret reconciled successfully")
		return ctrl.Result{Requeue: true}, nil
	}
	log.Info("Secret is up to date already, moving on...")

	deploy, err := r.getDeployment(wireguard)
	if err != nil {
		log.Error(err, "Cannot generate deployment")
		return ctrl.Result{}, err
	}
	if created, err := r.createIfNotExists(wireguard, ctx, deploy); err != nil {
		log.Error(err, "Cannot create deployment")
		return ctrl.Result{}, err
	} else if created {
		log.Info("Deployment created successfully")
		return ctrl.Result{Requeue: true}, nil
	}
	log.Info("Deployment is created already, moving on...")

	return ctrl.Result{}, nil
}

func (r *WireguardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpnv1alpha1.Wireguard{}).
		Owns(&vpnv1alpha1.WireguardPeer{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *WireguardReconciler) getService(wg *vpnv1alpha1.Wireguard) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        wg.Name,
			Namespace:   wg.Namespace,
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
}

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

func (r *WireguardReconciler) getConfigMap(
	wg *vpnv1alpha1.Wireguard) (*corev1.ConfigMap, error) {

	unboundTemplate, err := template.New("unbound").Parse(unboundConfTmpl)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err := unboundTemplate.Execute(buf, wg.Spec); err != nil {
		return nil, err
	}
	unboundConf := buf.String()

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wg.Name,
			Namespace: wg.Namespace,
		},
		Data: map[string]string{
			"unbound.conf": unboundConf,
		},
	}

	return configMap, nil
}

func (r *WireguardReconciler) getPeers(
	wg *vpnv1alpha1.Wireguard, ctx context.Context) (vpnv1alpha1.WireguardPeerList, error) {

	var allPeers vpnv1alpha1.WireguardPeerList
	err := r.List(ctx, &allPeers)
	if err != nil {
		return vpnv1alpha1.WireguardPeerList{}, err
	}

	peers := vpnv1alpha1.WireguardPeerList{
		Items: []vpnv1alpha1.WireguardPeer{},
	}
	for _, peer := range allPeers.Items {
		if peer.Spec.WireguardRef == wg.GetName() {
			peers.Items = append(peers.Items, peer)
		}
	}

	return peers, nil
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
{{ range .Peers }}
[Peer]
PublicKey = {{ .PublicKey }}
AllowedIPs = {{ .Address }}/32
Endpoint = {{ .Endpoint }}
PersistentKeepalive = 25
{{ end }}`

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

func (r *WireguardReconciler) getSecret(
	wireguard *vpnv1alpha1.Wireguard, peers vpnv1alpha1.WireguardPeerList, serviceIp string) (*corev1.Secret, error) {

	var privateKey, publicKey []byte
	ctx := context.TODO()

	current := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      wireguard.GetName(),
		Namespace: wireguard.GetNamespace(),
	}
	err := r.Get(ctx, key, current)
	if apierrors.IsNotFound(err) {
		// we need to create a new secret
		key, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			return nil, err
		}

		privateKey = []byte(key.String())
		publicKey = []byte(key.PublicKey().String())
	} else if err != nil {
		// unexpected error
		return nil, err
	} else {
		// assuming that secret exists
		privateKey = current.Data["private-key"]
		publicKey = current.Data["public-key"]
	}

	var wireguardPeers []serverPeer
	peerEndpoint := fmt.Sprintf("%s:%d", serviceIp, wireguardPort)
	for _, peer := range peers.Items {
		key := types.NamespacedName{
			Name:      peer.GetName(),
			Namespace: peer.GetNamespace(),
		}
		secret := &corev1.Secret{}
		if err := r.Get(ctx, key, secret); apierrors.IsNotFound(err) {
			// assuming that peer is not reconciled yet, so skip it
			continue
		} else if err != nil {
			// unexpected error
			return nil, err
		}

		wireguardPeers = append(wireguardPeers, serverPeer{
			PublicKey: string(secret.Data["public-key"]),
			Address:   peer.Spec.Address,
			Endpoint:  peerEndpoint,
		})
	}

	spec := serverConfig{
		Address:           wireguard.Spec.Network,
		PrivateKey:        string(privateKey),
		ListenPort:        wireguardPort,
		DropConnectionsTo: wireguard.Spec.DropConnectionsTo,
		Peers:             wireguardPeers,
	}
	tmpl, err := template.New("config").Parse(serverConfigTemplate)
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, spec); err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wireguard.Name,
			Namespace: wireguard.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"config":      buf.Bytes(),
			"public-key":  publicKey,
			"private-key": privateKey,
		},
	}

	return secret, nil
}

type containerMounts struct {
	unbound   []corev1.VolumeMount
	wireguard []corev1.VolumeMount
}

// getDeployment returns a Wireguard Deployment object
func (r *WireguardReconciler) getDeployment(
	wireguard *vpnv1alpha1.Wireguard) (*appsv1.Deployment, error) {

	volumes, mounts := getVolumes(wireguard)

	wireguardContainer := corev1.Container{
		Image:           getWireguardImage(),
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

	unboundContainer := corev1.Container{
		Image:           wireguard.Spec.DNS.Image,
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

	podSpec := &corev1.PodSpec{
		Affinity: wireguard.Spec.Affinity,
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
		Containers: []corev1.Container{
			wireguardContainer,
		},
		Volumes: volumes,
	}
	if wireguard.Spec.DNS.DeployServer {
		podSpec.DNSPolicy = corev1.DNSNone
		podSpec.DNSConfig = &corev1.PodDNSConfig{
			Nameservers: []string{"127.0.0.1"},
		}
		podSpec.Containers = append(podSpec.Containers, unboundContainer)
	}
	podSpec.Containers = append(podSpec.Containers, wireguard.Spec.Sidecars...)

	replicas := wireguard.Spec.Replicas
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wireguard.Name,
			Namespace: wireguard.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(wireguard.Name),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: getLabels(wireguard.Name),
				},
				Spec: *podSpec,
			},
		},
	}
	return dep, nil
}

func getVolumes(wireguard *vpnv1alpha1.Wireguard) ([]corev1.Volume, containerMounts) {
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

// creates resource if it doesn't exist. returns true when created
func (r *WireguardReconciler) createIfNotExists(
	wg *vpnv1alpha1.Wireguard, ctx context.Context, obj client.Object) (bool, error) {

	if err := ctrl.SetControllerReference(wg, obj, r.Scheme); err != nil {
		return false, err
	}

	key := types.NamespacedName{
		Name:      wg.Name,
		Namespace: wg.Namespace,
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

// creates or updates resource. returns true when created or updated
func (r *WireguardReconciler) createOrUpdateService(
	wg *vpnv1alpha1.Wireguard, ctx context.Context, desired *corev1.Service) (bool, error) {

	if err := ctrl.SetControllerReference(wg, desired, r.Scheme); err != nil {
		return false, err
	}

	// check if resource already exists
	key := types.NamespacedName{
		Name:      wg.Name,
		Namespace: wg.Namespace,
	}
	current := &corev1.Service{}
	err := r.Get(ctx, key, current)
	unexpectedError := err != nil && !apierrors.IsNotFound(err)
	if unexpectedError {
		return false, err
	}

	// annotate newly created resource
	annotator := patch.NewAnnotator("vpn.ahova.com/last-applied")
	if err := annotator.SetLastAppliedAnnotation(desired); err != nil {
		return false, err
	}

	// creating new resource
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

// creates or updates resource. returns true when created or updated
func (r *WireguardReconciler) createOrUpdateSecret(
	wg *vpnv1alpha1.Wireguard, ctx context.Context, desired *corev1.Secret) (bool, error) {

	if err := ctrl.SetControllerReference(wg, desired, r.Scheme); err != nil {
		return false, err
	}

	// check if resource already exists
	key := types.NamespacedName{
		Name:      wg.Name,
		Namespace: wg.Namespace,
	}
	current := &corev1.Secret{}
	err := r.Get(ctx, key, current)
	unexpectedError := err != nil && !apierrors.IsNotFound(err)
	if unexpectedError {
		return false, err
	}

	// annotate newly created resource
	annotator := patch.NewAnnotator("vpn.ahova.com/last-applied")
	if err := annotator.SetLastAppliedAnnotation(desired); err != nil {
		return false, err
	}

	// creating new resource
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

// getLabels returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func getLabels(name string) map[string]string {
	imageTag := strings.Split(getWireguardImage(), ":")[1]
	return map[string]string{
		"app.kubernetes.io/name":       "Wireguard",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "wireguard-operator",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

// getWireguardImage returns image for wireguard container
func getWireguardImage() string {
	return "linuxserver/wireguard:1.0.20210914"
}

func toPtr[V any](o V) *V { return &o }
