package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

const (
	timeout  = 10 * time.Second
	interval = 200 * time.Millisecond
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

	cases := []testCase{{
		context: "default configuration",
		wireguard: &vpnv1alpha1.Wireguard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wireguard-default",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: vpnv1alpha1.WireguardSpec{},
		},
	}, {
		context: "internal DNS configuration",
		wireguard: &vpnv1alpha1.Wireguard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wireguard-internal-dns",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: vpnv1alpha1.WireguardSpec{
				ExternalDNS: vpnv1alpha1.ExternalDNS{
					Enabled: false,
				},
			},
		},
	}, {
		context: "configuration with sidecars",
		wireguard: &vpnv1alpha1.Wireguard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wireguard-sidecars",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: vpnv1alpha1.WireguardSpec{
				Sidecars: []corev1.Container{sidecar},
			},
		},
	}}

	for _, spec := range cases {
		Context(spec.context, func() {
			It("should reconcile", testReconcile(spec.wireguard))
		})
	}
})

type testCase struct {
	context   string
	wireguard *vpnv1alpha1.Wireguard
}

// Performs full reconcildation loop for wireguard
func reconcileWireguard(ctx context.Context, key types.NamespacedName) error {
	reconciler := &WireguardReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
	// Reconcile resource multiple times to ensure that all resources are
	// created
	for i := 0; i < 5; i++ {
		req := reconcile.Request{
			NamespacedName: key,
		}
		if _, err := reconciler.Reconcile(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// Validates Wireguard resource and all dependent resources
func testReconcile(wireguard *vpnv1alpha1.Wireguard) func() {
	return func() {
		By("Setting prerequisites")
		key := types.NamespacedName{
			Name:      wireguard.ObjectMeta.Name,
			Namespace: wireguard.ObjectMeta.Namespace,
		}
		ctx := context.Background()

		By("Creating the custom resource for the Kind Wireguard")
		Expect(k8sClient.Create(ctx, wireguard)).To(Succeed())

		By("Checking if the custom resource was successfully created")
		Eventually(func() error {
			found := &vpnv1alpha1.Wireguard{}
			return k8sClient.Get(ctx, key, found)
		}, timeout, interval).Should(Succeed())

		By("Reconciling the custom resource created")
		Expect(reconcileWireguard(ctx, key)).To(Succeed())

		By("Checking if ConfigMap was successfully created in the reconciliation")
		Eventually(func() error {
			found := &corev1.ConfigMap{}
			return k8sClient.Get(ctx, key, found)
		}, timeout, interval).Should(Succeed())

		By("Checking if Secret was successfully created in the reconciliation")
		Eventually(func() error {
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, key, secret); err != nil {
				return err
			}

			Expect(secret.Data).To(HaveKey("wg-server"))
			Expect(secret.Data).To(HaveKey("wg-client"))

			return nil
		}, timeout, interval).Should(Succeed())

		By("Checking if Deployment was successfully created in the reconciliation")
		Eventually(func() error {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, key, deploy)
			if err != nil {
				return err
			}

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
			Expect(wg.SecurityContext).To(BeEquivalentTo(context))

			gotSysctls := deploy.Spec.Template.Spec.SecurityContext.Sysctls
			wantSysctls := []corev1.Sysctl{{
				Name:  "net.ipv4.ip_forward",
				Value: "1",
			}}
			Expect(gotSysctls).To(BeEquivalentTo(wantSysctls))

			dnsConfig := deploy.Spec.Template.Spec.DNSConfig
			dnsPolicy := deploy.Spec.Template.Spec.DNSPolicy
			volumes := deploy.Spec.Template.Spec.Volumes
			sidecarsLen := len(wireguard.Spec.Sidecars)
			if wireguard.Spec.ExternalDNS.Enabled {
				Expect(len(containers)).To(Equal(2 + sidecarsLen))
				Expect(len(volumes)).To(Equal(2))
				want := &corev1.PodDNSConfig{
					Nameservers: []string{"127.0.0.1"},
				}
				Expect(dnsConfig).To(BeEquivalentTo(want))
				Expect(dnsPolicy).To(Equal(corev1.DNSNone))
			} else {
				Expect(len(containers)).To(Equal(1 + sidecarsLen))
				Expect(len(volumes)).To(Equal(1))
				want := &corev1.PodDNSConfig{}
				Expect(dnsConfig).To(BeEquivalentTo(want))
				Expect(dnsPolicy).To(Equal(corev1.DNSDefault))
			}

			return nil
		}, timeout, interval).Should(Succeed())

		By("Checking if Service was successfully created in the reconciliation")
		Eventually(func() error {
			found := &corev1.Service{}
			return k8sClient.Get(ctx, key, found)
		}, timeout, interval).Should(Succeed())

		By("Checking the latest Status Condition added to the Wireguard instance")
		Eventually(func() error {
			conditions := wireguard.Status.Conditions
			if conditions != nil && len(conditions) != 0 {
				got := conditions[len(conditions)-1]
				msg := fmt.Sprintf(
					"Deployment for custom resource (%s) with %d replicas created successfully",
					wireguard.Name, wireguard.Spec.Replicas)
				want := metav1.Condition{
					Type:    typeAvailableWireguard,
					Status:  metav1.ConditionTrue,
					Reason:  "Reconciling",
					Message: msg,
				}
				if got != want {
					return fmt.Errorf("The latest status condition added to the wireguard instance is not as expected")
				}
			}
			return nil
		}, timeout, interval).Should(Succeed())
	}
}

var _ = DescribeTable("getLastIpInSubnet",
	func(input, want string) {
		got := getLastIpInSubnet(input)
		Expect(got).To(Equal(want))
	},
	Entry("smol", "192.168.254.253/30", "192.168.254.254/32"),
	Entry("chungus", "192.168.1.1/24", "192.168.1.254/32"),
)

var _ = DescribeTable("getFirstIpInSubnet",
	func(input, want string) {
		got := getFirstIpInSubnet(input)
		Expect(got).To(Equal(want))
	},
	Entry("smol", "192.168.254.253/30", "192.168.254.253/32"),
	Entry("chungus", "192.168.1.1/24", "192.168.1.1/32"),
)
