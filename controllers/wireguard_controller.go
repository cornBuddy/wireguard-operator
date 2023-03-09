package controllers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/c-robinson/iplib"
	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

// Definitions to manage status conditions
const (
	// typeAvailableWireguard represents the status of the Deployment reconciliation
	typeAvailableWireguard = "Available"
)

// WireguardReconciler reconciles a Wireguard object
type WireguardReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// The following markers are used to generate the rules permissions (RBAC) on config/rbac using controller-gen
// when the command <make manifests> is executed.
// To know more about markers see: https://book.kubebuilder.io/reference/markers.html

//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpn.ahova.com,resources=wireguards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// It is essential for the controller's reconciliation loop to be idempotent. By following the Operator
// pattern you will create Controllers which provide a reconcile function
// responsible for synchronizing resources until the desired state is reached on the cluster.
// Breaking this recommendation goes against the design principles of controller-runtime.
// and may lead to unforeseen consequences such as resources becoming stuck and requiring manual intervention.
// For further info:
// - About Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
// - About Controllers: https://kubernetes.io/docs/concepts/architecture/controller/
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *WireguardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Wireguard instance
	// The purpose is check if the Custom Resource for the Kind Wireguard
	// is applied on the cluster if not we return nil to stop the reconciliation
	wireguard := &vpnv1alpha1.Wireguard{}
	err := r.Get(ctx, req.NamespacedName, wireguard)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("wireguard resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get wireguard")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	statusIsEmpty := wireguard.Status.Conditions == nil || len(wireguard.Status.Conditions) == 0
	if statusIsEmpty {
		condition := metav1.Condition{
			Type:    typeAvailableWireguard,
			Status:  metav1.ConditionUnknown,
			Reason:  "Reconciling",
			Message: "Starting reconciliation",
		}
		meta.SetStatusCondition(&wireguard.Status.Conditions, condition)
		if err = r.Status().Update(ctx, wireguard); err != nil {
			log.Error(err, "Failed to update Wireguard status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the wireguard Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, wireguard); err != nil {
			log.Error(err, "Failed to re-fetch wireguard")
			return ctrl.Result{}, err
		}
	}

	key := types.NamespacedName{
		Name:      wireguard.Name,
		Namespace: wireguard.Namespace,
	}

	// Check if the confgimap already exists, if not create a new one
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, key, configMap); err == nil {
		log.Info("Ensured that ConfigMap is created",
			"Wireguard.Namespace", wireguard.Namespace,
			"Wireguard.Name", wireguard.Name)
	} else if apierrors.IsNotFound(err) {
		log.Info("Creating ConfigMap for",
			"Wireguard.Namespace", wireguard.Namespace,
			"Wireguard.Name", wireguard.Name)
		if err := r.createConfigMap(wireguard, ctx); err != nil {
			log.Error(err, "Cannot create ConfigMap for",
				"Wireguard.Namespace", wireguard.Namespace,
				"Wireguard.Name", wireguard.Name)
			return ctrl.Result{}, err
		} else {
			log.Info("ConfigMap created successfully for",
				"Wireguard.Namespace", wireguard.Namespace,
				"Wireguard.Name", wireguard.Name)
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		return ctrl.Result{}, err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, key, service); err == nil {
		log.Info("Ensured that Service is created",
			"Wireguard.Namespace", wireguard.Namespace,
			"Wireguard.Name", wireguard.Name)
	} else if apierrors.IsNotFound(err) {
		log.Info("Creating Service for",
			"Wireguard.Namespace", wireguard.Namespace,
			"Wireguard.Name", wireguard.Name)
		if err := r.createService(wireguard, ctx); err != nil {
			log.Error(err, "Cannot create Service for",
				"Wireguard.Namespace", wireguard.Namespace,
				"Wireguard.Name", wireguard.Name)
			return ctrl.Result{}, err
		} else {
			log.Info("Service created successfully for",
				"Wireguard.Namespace", wireguard.Namespace,
				"Wireguard.Name", wireguard.Name)
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		return ctrl.Result{}, err
	}

	// Check if secret already exists. If not, create a new one
	secret := &corev1.Secret{}
	if err := r.Get(ctx, key, secret); err == nil {
		log.Info("Ensured that Secret is created",
			"Wireguard.Namespace", wireguard.Namespace,
			"Wireguard.Name", wireguard.Name)
	} else if apierrors.IsNotFound(err) {
		log.Info("Creating Secret for",
			"Wireguard.Namespace", wireguard.Namespace,
			"Wireguard.Name", wireguard.Name)
		serviceIp := service.Spec.ClusterIP
		if err := r.createSecret(wireguard, serviceIp, ctx); err != nil {
			log.Error(err, "Cannot create Secret for",
				"Wireguard.Namespace", wireguard.Namespace,
				"Wireguard.Name", wireguard.Name)
			return ctrl.Result{}, err
		} else {
			log.Info("Secret created successfully for",
				"Wireguard.Namespace", wireguard.Namespace,
				"Wireguard.Name", wireguard.Name)
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	deploy := &appsv1.Deployment{}
	err = r.Get(ctx, key, deploy)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := r.getDeployment(wireguard)
		if err != nil {
			log.Error(err, "Failed to define new Deployment resource for Wireguard")

			// The following implementation will update the status
			condition := metav1.Condition{
				Type:    typeAvailableWireguard,
				Status:  metav1.ConditionFalse,
				Reason:  "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", wireguard.Name, err),
			}
			meta.SetStatusCondition(&wireguard.Status.Conditions, condition)

			if err := r.Status().Update(ctx, wireguard); err != nil {
				log.Error(err, "Failed to update Wireguard status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			log.Error(err, "Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// The CRD API is defining that the Wireguard type, have a WireguardSpec.Replicas field
	// to set the quantity of Deployment instances is the desired state on the cluster.
	// Therefore, the following code will ensure the Deployment size is the same as defined
	// via the Replicas spec of the Custom Resource which we are reconciling.
	size := wireguard.Spec.Replicas
	if *deploy.Spec.Replicas != size {
		deploy.Spec.Replicas = &size
		if err = r.Update(ctx, deploy); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", deploy.Namespace, "Deployment.Name", deploy.Name)

			// Re-fetch the wireguard Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, wireguard); err != nil {
				log.Error(err, "Failed to re-fetch wireguard")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			cdnt := metav1.Condition{
				Type:   typeAvailableWireguard,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", wireguard.Name, err),
			}
			meta.SetStatusCondition(&wireguard.Status.Conditions, cdnt)

			if err := r.Status().Update(ctx, wireguard); err != nil {
				log.Error(err, "Failed to update Wireguard status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	// The following implementation will update the status
	msg := fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", wireguard.Name, size)
	cdnt := metav1.Condition{
		Type:    typeAvailableWireguard,
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciling",
		Message: msg,
	}
	meta.SetStatusCondition(&wireguard.Status.Conditions, cdnt)
	if err := r.Status().Update(ctx, wireguard); err != nil {
		log.Error(err, "Failed to update Wireguard status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *WireguardReconciler) getService(
	wireguard *vpnv1alpha1.Wireguard) (*corev1.Service, error) {
	ls := getLabels(wireguard.Name)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wireguard.Name,
			Namespace: wireguard.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: ls,
			Ports: []corev1.ServicePort{{
				Name:     "wireguard",
				Protocol: "UDP",
				Port:     wireguard.Spec.ContainerPort,
			}},
		},
	}

	if err := ctrl.SetControllerReference(wireguard, service, r.Scheme); err != nil {
		return nil, err
	}
	return service, nil
}

func (r *WireguardReconciler) createService(
	wireguard *vpnv1alpha1.Wireguard, ctx context.Context) error {

	service, err := r.getService(wireguard)
	if err != nil {
		msg := fmt.Sprintf("Failed to create Service for the custom resource (%s): (%s)", wireguard.Name, err)
		cdtn := metav1.Condition{
			Type:    typeAvailableWireguard,
			Status:  metav1.ConditionFalse,
			Reason:  "Reconciling",
			Message: msg,
		}
		meta.SetStatusCondition(&wireguard.Status.Conditions, cdtn)
		if err := r.Status().Update(ctx, wireguard); err != nil {
			return err
		}
		return err
	}
	if err := r.Create(ctx, service); err != nil {
		return err
	}
	return nil
}

func (r *WireguardReconciler) createConfigMap(
	wireguard *vpnv1alpha1.Wireguard, ctx context.Context) error {

	cm, err := r.getConfigMap(wireguard)
	if err != nil {
		msg := fmt.Sprintf("Failed to create ConfigMap for the custom resource (%s): (%s)", wireguard.Name, err)
		condition := metav1.Condition{
			Type:    typeAvailableWireguard,
			Status:  metav1.ConditionFalse,
			Reason:  "Reconciling",
			Message: msg,
		}
		meta.SetStatusCondition(&wireguard.Status.Conditions, condition)
		if err := r.Status().Update(ctx, wireguard); err != nil {
			return err
		}
		return err
	}
	if err = r.Create(ctx, cm); err != nil {
		return err
	}
	return nil
}

// getConfigMap returns a Wireguard ConfigMap object
func (r *WireguardReconciler) getConfigMap(
	wireguard *vpnv1alpha1.Wireguard) (*corev1.ConfigMap, error) {

	unboundTemplate, err := template.New("unbound").Parse(unboundConfTmpl)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err := unboundTemplate.Execute(buf, wireguard.Spec); err != nil {
		return nil, err
	}
	unboundConf := buf.String()

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wireguard.Name,
			Namespace: wireguard.Namespace,
		},
		Data: map[string]string{
			"unbound.conf": unboundConf,
		},
	}

	if err := ctrl.SetControllerReference(wireguard, configMap, r.Scheme); err != nil {
		return nil, err
	}

	return configMap, nil
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
	access-control: {{ .Address }} allow
	private-address: {{ .Address }}
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

func (r *WireguardReconciler) createSecret(
	wireguard *vpnv1alpha1.Wireguard, serviceIp string, ctx context.Context) error {

	server, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return err
	}
	peer, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return err
	}

	secret, err := r.getSecret(wireguard, serviceIp, server, peer)
	if err != nil {
		msg := fmt.Sprintf("Failed to create Secret for the custom resource (%s): (%s)", wireguard.Name, err)
		condition := metav1.Condition{
			Type:    typeAvailableWireguard,
			Status:  metav1.ConditionFalse,
			Reason:  "Reconciling",
			Message: msg,
		}
		meta.SetStatusCondition(&wireguard.Status.Conditions, condition)
		if err := r.Status().Update(ctx, wireguard); err != nil {
			return err
		}
		return err
	}

	if err := r.Create(ctx, secret); err != nil {
		return err
	}

	return nil
}

func (r *WireguardReconciler) getSecret(
	wireguard *vpnv1alpha1.Wireguard, serviceIp string, server, peer wgtypes.Key) (*corev1.Secret, error) {

	keys := []string{"wg-server", "wg-client"}
	configs := map[string]string{
		"wg-server": serverConfig,
		"wg-client": peerConfig,
	}
	peerAddress := getLastIpInSubnet(wireguard.Spec.Address)
	// FIXME: do not use container port
	port := wireguard.Spec.ContainerPort
	peerEndpoint := fmt.Sprintf("%s:%d", serviceIp, port)
	ep := wireguard.Spec.EndpointAddress
	serverEndpoint := fmt.Sprintf("%s:%d", ep, port)
	dns := getFirstIpInSubnet(wireguard.Spec.Address)
	specs := map[string]any{
		"wg-server": serverSpec{
			Address:       wireguard.Spec.Address,
			PrivateKey:    server.String(),
			ListenPort:    wireguard.Spec.ContainerPort,
			PeerPublicKey: peer.PublicKey().String(),
			PeerAddress:   peerAddress,
			PeerEndpoint:  peerEndpoint,
		},
		"wg-client": clientSpec{
			Address:         peerAddress,
			PrivateKey:      peer.String(),
			ServerPublicKey: server.PublicKey().String(),
			ServerEndpoint:  serverEndpoint,
			DNS:             dns,
		},
	}
	data := map[string]string{}

	for _, key := range keys {
		tmpl, err := template.New(key).Parse(configs[key])
		if err != nil {
			return nil, err
		}

		buf := new(bytes.Buffer)
		if err := tmpl.Execute(buf, specs[key]); err != nil {
			return nil, err
		}

		data[key] = buf.String()
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wireguard.Name,
			Namespace: wireguard.Namespace,
		},
		StringData: data,
	}

	if err := ctrl.SetControllerReference(wireguard, secret, r.Scheme); err != nil {
		return nil, err
	}

	return secret, nil
}

type serverSpec struct {
	Address       string
	PrivateKey    string
	ListenPort    int32
	PeerPublicKey string
	PeerAddress   string
	PeerEndpoint  string
}

type clientSpec struct {
	Address         string
	PrivateKey      string
	DNS             string
	ServerPublicKey string
	ServerEndpoint  string
}

// represents go-template for the server config
const serverConfig = `[Interface]
Address = {{ .Address }}
PrivateKey = {{ .PrivateKey }}
ListenPort = {{ .ListenPort }}
PostUp = iptables --append FORWARD --in-interface %i --jump ACCEPT
PostUp = iptables --append FORWARD --out-interface %i --jump ACCEPT
PostUp = iptables --table nat --append POSTROUTING --source {{ .PeerAddress }} --out-interface eth0 --jump MASQUERADE

[Peer]
PublicKey = {{ .PeerPublicKey }}
AllowedIPs = {{ .PeerAddress }}
Endpoint = {{ .PeerEndpoint }}
PersistentKeepalive = 25`

const peerConfig = `[Interface]
Address = {{ .Address }}
PrivateKey = {{ .PrivateKey }}
DNS = {{ .DNS }}

[Peer]
PublicKey = {{ .ServerPublicKey }}
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = {{ .ServerEndpoint }}`

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
			ContainerPort: wireguard.Spec.ContainerPort,
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
		Image:           wireguard.Spec.ExternalDNS.Image,
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
		SecurityContext: &corev1.PodSecurityContext{
			Sysctls: []corev1.Sysctl{{
				Name:  "net.ipv4.ip_forward",
				Value: "1",
			}},
		},
		Containers: []corev1.Container{
			wireguardContainer,
		},
		Volumes: volumes,
	}
	if wireguard.Spec.ExternalDNS.Enabled {
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

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(wireguard, dep, r.Scheme); err != nil {
		return nil, err
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
					Key:  "wg-server",
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
	if wireguard.Spec.ExternalDNS.Enabled {
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

// getWireguardImage gets the Operand image which is managed by this controller
// from the WIREGUARD_IMAGE environment variable defined in the config/manager/manager.yaml
func getWireguardImage() string {
	image, found := os.LookupEnv("WIREGUARD_IMAGE")
	if !found {
		return "linuxserver/wireguard:1.0.20210914"
	}
	return image
}

// SetupWithManager sets up the controller with the Manager.
// Note that the Deployment will be also watched in order to ensure its
// desirable state on the cluster
func (r *WireguardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpnv1alpha1.Wireguard{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

type containerMounts struct {
	unbound   []corev1.VolumeMount
	wireguard []corev1.VolumeMount
}

func toPtr[V any](o V) *V { return &o }

// returns last ip in the subnet. examples:
// getLastIpInSubnet("192.168.254.253/30") -> "192.168.254.254/32"
// getLastIpInSubnet("192.168.1.1/24") -> "192.168.1.254/32"
func getLastIpInSubnet(cidr string) string {
	// ignore error since cidr should be validated already
	_, net, _ := iplib.ParseCIDR(cidr)
	last := net.LastAddress().String()
	return fmt.Sprintf("%s/32", last)
}

// returns first ip in the subnet. examples:
// getLastIpInSubnet("192.168.254.253/30") -> "192.168.254.253/32"
// getLastIpInSubnet("192.168.1.1/24") -> "192.168.1.1/32"
func getFirstIpInSubnet(cidr string) string {
	// ignore error since cidr should be validated already
	_, net, _ := iplib.ParseCIDR(cidr)
	last := net.FirstAddress().String()
	return fmt.Sprintf("%s/32", last)
}
