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
	typeTestCases := []TableEntry{
		Entry(nil, corev1.ServiceTypeClusterIP),
		Entry(nil, corev1.ServiceTypeLoadBalancer),
	}
	updatableTestCases := []TableEntry{
		Entry(
			"can update .spec.type",
			vpnv1alpha1.WireguardSpec{ServiceType: corev1.ServiceTypeClusterIP},
			vpnv1alpha1.WireguardSpec{ServiceType: corev1.ServiceTypeLoadBalancer},
		),
	}

	DescribeTable("has valid type", func(st corev1.ServiceType) {
		By("provisioning wireguard CRD")
		wireguard := testdsl.GenerateWireguard(vpnv1alpha1.WireguardSpec{
			ServiceType: st,
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
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, key, svc)).To(Succeed())
		Expect(svc).ToNot(BeNil())

		By("validating")
		Expect(wireguard.Spec.ServiceType).To(Equal(st))
		Expect(svc.Spec.Type).To(Equal(st))
	}, typeTestCases)

	DescribeTable("is updatable", func(spec1, spec2 vpnv1alpha1.WireguardSpec) {
		By("setting prerequisites")
		wg1 := testdsl.GenerateWireguard(spec1)
		key := types.NamespacedName{
			Name:      wg1.GetName(),
			Namespace: wg1.GetNamespace(),
		}
		svc1 := &corev1.Service{}
		svc2 := &corev1.Service{}

		By("creating initial resource")
		Eventually(func() error {
			return k8sClient.Create(ctx, wg1)
		}, timeout, interval).Should(Succeed())
		Expect(wgDsl.Reconcile(wg1)).To(Succeed())

		By("fetching original service")
		Eventually(func() error {
			return k8sClient.Get(ctx, key, svc1)
		}, timeout, interval).Should(Succeed())

		By("updating resource")
		wg2 := &vpnv1alpha1.Wireguard{}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, wg2)
		}, timeout, interval).Should(Succeed())
		wg2.Spec = spec2
		Eventually(func() error {
			return k8sClient.Update(ctx, wg2)
		}, timeout, interval).Should(Succeed())
		Expect(wgDsl.Reconcile(wg2)).To(Succeed())

		By("fetching updated service")
		Expect(k8sClient.Get(ctx, key, svc2)).To(Succeed())

		By("validating")
		Expect(svc1.Spec.Type).To(Equal(spec1.ServiceType))
		Expect(svc2.Spec.Type).To(Equal(spec2.ServiceType))
	}, updatableTestCases)
})
