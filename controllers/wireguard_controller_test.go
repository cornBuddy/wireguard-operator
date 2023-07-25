package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
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
		func(wg *vpnv1alpha1.Wireguard) {
			By("reconciling wireguard CR")
			validateReconcile(wg, wgDsl)

			By("reconciling peers CRs")
			peerName := names.SimpleNameGenerator.GenerateName("peer-")
			peer := &vpnv1alpha1.WireguardPeer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      peerName,
					Namespace: corev1.NamespaceDefault,
				},
				Spec: vpnv1alpha1.WireguardPeerSpec{
					WireguardRef: wg.GetName(),
				},
			}
			validateReconcile(peer, peerDsl)

			By("reconciling wireguard CR once again")
			Expect(wgDsl.Reconcile(wg)).To(Succeed())

			By("fetching the list of peers")
			wgReconciler := &WireguardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			peers, err := wgReconciler.getPeers(wg, ctx)
			Expect(err).To(BeNil())
			Expect(peers.Items).To(HaveLen(1))

			By("validating wireguard CR")
			validateConfigMap(wg)
			validateDeployment(wg)
		},
		Entry(
			"default configuration",
			testdsl.GenerateWireguard(vpnv1alpha1.WireguardSpec{}),
		),
		Entry(
			"internal dns configuration",
			testdsl.GenerateWireguard(vpnv1alpha1.WireguardSpec{
				DNS: vpnv1alpha1.DNS{
					DeployServer: false,
					Address:      "10.96.0.1",
				},
			}),
		),
		Entry(
			"sidecar configuration",
			testdsl.GenerateWireguard(vpnv1alpha1.WireguardSpec{
				Sidecars: []corev1.Container{sidecar},
			}),
		),
	)
})

func validateConfigMap(wireguard *vpnv1alpha1.Wireguard) {
	key := types.NamespacedName{
		Name:      wireguard.GetName(),
		Namespace: wireguard.GetNamespace(),
	}
	cm := &corev1.ConfigMap{}

	By("Checking if ConfigMap was successfully created in the reconciliation")
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, key, cm)).To(Succeed())
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
		g.Expect(k8sClient.Get(ctx, key, deploy)).To(Succeed())

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
