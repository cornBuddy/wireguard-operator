package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var (
	defaultSpec   = v1alpha1.WireguardSpec{}
	deployDnsSpec = v1alpha1.WireguardSpec{
		DNS: v1alpha1.DNS{
			DeployServer: true,
		},
	}
)

var _ = Describe("Wireguard#Deployment", func() {
	var deploy *appsv1.Deployment

	BeforeEach(func() {
		deploy = &appsv1.Deployment{}
	})

	AfterEach(func() {
		deploy = &appsv1.Deployment{}
	})

	DescribeTable("should be created properly", func(spec v1alpha1.WireguardSpec, containersCount int) {
		By("reconcing wireguard instance")
		wg := testdsl.GenerateWireguard(spec)
		Eventually(func() error {
			return wgDsl.Apply(ctx, wg)
		}, timeout, interval).Should(Succeed())

		By("fetching deployment")
		key := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, deploy)
		}, timeout, interval).Should(Succeed())

		By("validating pod context")
		context := &corev1.SecurityContext{
			Privileged: toPtr(true),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN",
					"SYS_MODULE",
				},
			},
		}
		wgContainer := deploy.Spec.Template.Spec.Containers[0]
		Expect(wgContainer.SecurityContext).To(BeEquivalentTo(context))

		By("validating sysctls")
		gotSysctls := deploy.Spec.Template.Spec.SecurityContext.Sysctls
		wantSysctls := []corev1.Sysctl{{
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
		}}
		Expect(gotSysctls).To(BeEquivalentTo(wantSysctls))
	},
		Entry("with default spec", defaultSpec, 1),
		Entry("with dns spec", deployDnsSpec, 2),
	)

	It("should should have proper dns config when .dns.deployServer == true", func() {
		By("reconcing wireguard instance")
		wg := testdsl.GenerateWireguard(deployDnsSpec)
		Eventually(func() error {
			return wgDsl.Apply(ctx, wg)
		}, timeout, interval).Should(Succeed())

		By("fetching deployment")
		key := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, deploy)
		}, timeout, interval).Should(Succeed())

		By("validating dns config")
		dnsConfig := &corev1.PodDNSConfig{
			Nameservers: []string{"127.0.0.1"},
		}
		Expect(deploy.Spec.Template.Spec.DNSConfig).To(BeEquivalentTo(dnsConfig))
		Expect(deploy.Spec.Template.Spec.DNSPolicy).To(Equal(corev1.DNSNone))
	})

	It("should should have default dns config by default", func() {
		By("reconcing wireguard instance")
		wg := testdsl.GenerateWireguard(defaultSpec)
		Eventually(func() error {
			return wgDsl.Apply(ctx, wg)
		}, timeout, interval).Should(Succeed())

		By("fetching deployment")
		key := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, deploy)
		}, timeout, interval).Should(Succeed())

		By("validating dns config")
		Expect(deploy.Spec.Template.Spec.DNSConfig).To(BeNil())
		Expect(deploy.Spec.Template.Spec.DNSPolicy).To(Equal(corev1.DNSClusterFirst))
	})
})
