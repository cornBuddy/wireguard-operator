package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var _ = Describe("Wireguard#Service", func() {
	var wireguard *vpnv1alpha1.Wireguard
	svc := &corev1.Service{}

	BeforeEach(func() {
		By("provisioning wireguard CRD")
		wireguard = testdsl.GenerateWireguard(vpnv1alpha1.WireguardSpec{
			ServiceType: corev1.ServiceTypeLoadBalancer,
		})
		Eventually(func() error {
			return k8sClient.Create(ctx, wireguard)
		}, timeout, interval).Should(Succeed())
		Expect(wgDsl.Reconcile(wireguard)).To(Succeed())

		By("fetching service from cluster")
		key := types.NamespacedName{
			Name:      wireguard.GetName(),
			Namespace: wireguard.GetNamespace(),
		}
		Expect(k8sClient.Get(ctx, key, svc)).To(Succeed())
		Expect(svc).ToNot(BeNil())
	})

	It("should have valid service type", func() {
		Expect(wireguard.Spec.ServiceType).To(Equal(svc.Spec.Type))
	})
})
