package controllers

import (
	"crypto/sha1"
	"encoding/hex"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var (
	defaultSpec    = v1alpha1.WireguardSpec{}
	clusterDnsSpec = v1alpha1.WireguardSpec{
		DNS: &v1alpha1.DNS{
			DeployServer: false,
			Address:      "127.0.0.1",
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

	DescribeTable("should be created properly", func(spec v1alpha1.WireguardSpec) {
		By("reconcing wireguard instance")
		status := v1alpha1.WireguardStatus{}
		wg := testdsl.GenerateWireguard(spec, status)
		Eventually(func() error {
			return wgDsl.Apply(ctx, &wg)
		}, timeout, interval).Should(Succeed())

		By("fetching deployment")
		key := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, deploy)
		}, timeout, interval).Should(Succeed())

		By("validating pod security context")
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

		By("fetching corresponding secret")
		secret := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, secret)
		}, timeout, interval).Should(Succeed())

		By("checking pod template contains last applied config hash")
		config := secret.Data["config"]
		hash := sha1.New()
		hash.Write(config)
		configHash := hex.EncodeToString(hash.Sum(nil))
		podAnnotations := deploy.Spec.Template.Annotations
		Expect(podAnnotations).To(HaveKey("vpn.ahova.com/config-hash"))
		Expect(podAnnotations["vpn.ahova.com/config-hash"]).To(Equal(configHash))
	},
		Entry("with default spec", defaultSpec),
		Entry("with cluster dns spec", clusterDnsSpec),
	)
})
