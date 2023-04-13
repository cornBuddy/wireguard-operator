package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

var _ = Describe("Wireguard controller", func() {
	sidecar := corev1.Container{
		Name:  "wireguard-exporter",
		Image: "docker.io/mindflavor/prometheus-wireguard-exporter:3.6.6",
		Args: []string{
			"--verbose", "true",
			"--extract_names_config_files", "/config/wg0.conf",
		},
		VolumeMounts: []corev1.VolumeMount{{
			Name:      "wireguard-config",
			ReadOnly:  true,
			MountPath: "/config",
		}},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  toPtr[int64](0),
			RunAsGroup: toPtr[int64](0),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN",
				},
			},
		},
	}

	DescribeTable("should reconcile",
		func(wireguard *vpnv1alpha1.Wireguard) {
			By("reconciling wireguard CR")
			wgReconciler := &WireguardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			wireguardName := names.SimpleNameGenerator.GenerateName("wireguard-")
			wireguard.SetName(wireguardName)
			validateReconcile(wireguard, wgReconciler)

			By("reconciling peers CRs")
			peerReconciler := &WireguardPeerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			peerName := names.SimpleNameGenerator.GenerateName("peer-")
			peer := &vpnv1alpha1.WireguardPeer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      peerName,
					Namespace: corev1.NamespaceDefault,
				},
				Spec: vpnv1alpha1.WireguardPeerSpec{
					WireguardRef: wireguardName,
				},
			}
			validateReconcile(peer, peerReconciler)

			By("reconciling wireguard CR once again")
			key := types.NamespacedName{
				Name:      wireguard.GetName(),
				Namespace: wireguard.GetNamespace(),
			}
			Expect(reconcileCustomResource(key, wgReconciler)).To(Succeed())

			By("fetching the list of peers")
			peers, err := wgReconciler.getPeers(wireguard, context.TODO())
			Expect(err).To(BeNil())
			Expect(peers.Items).To(HaveLen(1))

			By("validating wireguard CR")
			validateWireguardSecret(wireguard, peers)
			validateService(wireguard)
			validateConfigMap(wireguard)
			validateDeployment(wireguard)
		},
		Entry(
			"default configuration",
			&vpnv1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
				},
				Spec: vpnv1alpha1.WireguardSpec{},
			},
		),
		Entry(
			"internal dns configuration",
			&vpnv1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
				},
				Spec: vpnv1alpha1.WireguardSpec{
					DNS: vpnv1alpha1.DNS{
						DeployServer: false,
						Address:      "10.96.0.1",
					},
				},
			},
		),
		Entry(
			"sidecar configuration",
			&vpnv1alpha1.Wireguard{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
				},
				Spec: vpnv1alpha1.WireguardSpec{
					Sidecars: []corev1.Container{sidecar},
				},
			},
		),
	)
})

func validateService(wireguard *vpnv1alpha1.Wireguard) {
	key := types.NamespacedName{
		Name:      wireguard.GetName(),
		Namespace: wireguard.GetNamespace(),
	}
	svc := &corev1.Service{}

	By("Checking if Service was successfully created in the reconciliation")
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(context.TODO(), key, svc)).To(Succeed())
	}, timeout, interval).Should(Succeed())
}

func validateConfigMap(wireguard *vpnv1alpha1.Wireguard) {
	key := types.NamespacedName{
		Name:      wireguard.GetName(),
		Namespace: wireguard.GetNamespace(),
	}
	cm := &corev1.ConfigMap{}

	By("Checking if ConfigMap was successfully created in the reconciliation")
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(context.TODO(), key, cm)).To(Succeed())
	}, timeout, interval).Should(Succeed())
}

func validateWireguardSecret(wireguard *vpnv1alpha1.Wireguard, peers vpnv1alpha1.WireguardPeerList) {
	key := types.NamespacedName{
		Name:      wireguard.GetName(),
		Namespace: wireguard.GetNamespace(),
	}
	secret := &corev1.Secret{}

	By("Checking if Secret was successfully created in the reconciliation")
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(context.TODO(), key, secret)).Should(Succeed())
		g.Expect(secret.Data).To(HaveKey("config"))
		g.Expect(secret.Data).To(HaveKey("public-key"))
		g.Expect(secret.Data).To(HaveKey("private-key"))

		annotations := secret.GetAnnotations()
		g.Expect(annotations).To(HaveKey("banzaicloud.com/last-applied"))

		const keyLength = 44
		g.Expect(secret.Data["public-key"]).To(HaveLen(keyLength))
		g.Expect(secret.Data["private-key"]).To(HaveLen(keyLength))
		privateKey := string(secret.Data["private-key"])
		wgKey, err := wgtypes.ParseKey(privateKey)
		g.Expect(err).To(BeNil())
		pubKey := wgKey.PublicKey().String()
		g.Expect(string(secret.Data["public-key"])).To(Equal(pubKey))

		cfg := string(secret.Data["config"])

		privKey := fmt.Sprintf("PrivateKey = %s", secret.Data["private-key"])
		g.Expect(cfg).To(ContainSubstring(privKey))

		for _, postUp := range getPostUps(wireguard) {
			g.Expect(cfg).To(ContainSubstring(postUp))
		}

		g.Expect(cfg).To(ContainSubstring("[Peer]"))

		for _, peer := range peers.Items {
			ip := fmt.Sprintf("AllowedIPs = %s/32", peer.Spec.Address)
			g.Expect(cfg).To(ContainSubstring(ip))
			peerKey := types.NamespacedName{
				Name:      peer.GetName(),
				Namespace: peer.GetNamespace(),
			}

			peerSecret := &corev1.Secret{}
			g.Expect(k8sClient.Get(context.TODO(), peerKey, peerSecret)).To(Succeed())
			pubKey := fmt.Sprintf("PublicKey = %s", peerSecret.Data["public-key"])
			g.Expect(cfg).To(ContainSubstring(pubKey))
		}

		address := fmt.Sprintf("Address = %s", wireguard.Spec.Network)
		g.Expect(cfg).To(ContainSubstring(address))
	}, timeout, interval).Should(Succeed())
}

func validateDeployment(wireguard *vpnv1alpha1.Wireguard) {
	By("Checking if Deployment was successfully created in the reconciliation")
	key := types.NamespacedName{
		Name:      wireguard.ObjectMeta.Name,
		Namespace: wireguard.ObjectMeta.Namespace,
	}
	deploy := &appsv1.Deployment{}

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(context.TODO(), key, deploy)).To(Succeed())

		context := &corev1.SecurityContext{
			Privileged: toPtr(true),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN",
					"SYS_MODULE",
				},
			},
		}
		containers := deploy.Spec.Template.Spec.Containers
		wg := containers[0]
		g.Expect(wg.SecurityContext).To(BeEquivalentTo(context))

		gotSysctls := deploy.Spec.Template.Spec.SecurityContext.Sysctls
		wantSysctls := []corev1.Sysctl{{
			Name:  "net.ipv4.ip_forward",
			Value: "1",
		}}
		g.Expect(gotSysctls).To(BeEquivalentTo(wantSysctls))

		dnsConfig := deploy.Spec.Template.Spec.DNSConfig
		dnsPolicy := deploy.Spec.Template.Spec.DNSPolicy
		volumes := deploy.Spec.Template.Spec.Volumes
		sidecarsLen := len(wireguard.Spec.Sidecars)

		var baseLen int
		if wireguard.Spec.DNS.DeployServer {
			baseLen = 2

			want := &corev1.PodDNSConfig{
				Nameservers: []string{"127.0.0.1"},
			}
			g.Expect(dnsConfig).To(BeEquivalentTo(want))
			g.Expect(dnsPolicy).To(Equal(corev1.DNSNone))
		} else {
			baseLen = 1

			g.Expect(dnsConfig).To(BeNil())
			g.Expect(dnsPolicy).To(Equal(corev1.DNSClusterFirst))
		}

		g.Expect(len(containers)).To(Equal(baseLen + sidecarsLen))
		g.Expect(len(volumes)).To(Equal(baseLen))
	}, timeout, interval).Should(Succeed())
}

func getPostUps(wireguard *vpnv1alpha1.Wireguard) []string {
	masquerade := fmt.Sprintf(
		"PostUp = iptables --table nat --append POSTROUTING --source %s --out-interface eth0 --jump MASQUERADE",
		wireguard.Spec.Network,
	)
	mandatoryPostUps := []string{
		"PostUp = iptables --append FORWARD --in-interface %i --jump ACCEPT",
		"PostUp = iptables --append FORWARD --out-interface %i --jump ACCEPT",
		masquerade,
	}
	hardeningPostUps := getHardeningPostUps(wireguard)
	postUps := append(mandatoryPostUps, hardeningPostUps...)
	return postUps
}

func getHardeningPostUps(wireguard *vpnv1alpha1.Wireguard) []string {
	var postUps []string
	for _, dest := range wireguard.Spec.DropConnectionsTo {
		postUp := fmt.Sprintf(
			"PostUp = iptables --insert FORWARD --source %s --destination %s --jump DROP",
			wireguard.Spec.Network,
			dest,
		)
		postUps = append(postUps, postUp)
	}
	return postUps
}
