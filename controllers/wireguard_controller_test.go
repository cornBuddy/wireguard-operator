package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
