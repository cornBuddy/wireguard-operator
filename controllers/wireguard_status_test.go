package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var _ = Describe("Wireguard.Status", func() {
	It("should set public key same as in the secret", func() {
		By("applying CRDs")
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{}, v1alpha1.WireguardStatus{})
		key := types.NamespacedName{
			Namespace: wg.GetNamespace(),
			Name:      wg.GetName(),
		}
		Eventually(func() error {
			return wgDsl.Apply(ctx, &wg)
		}, timeout, interval).Should(Succeed())

		By("refetching CRD from cluster")
		Eventually(func() error {
			return k8sClient.Get(ctx, key, &wg)
		}, timeout, interval).Should(Succeed())
		Expect(wg.Status.PublicKey).ToNot(BeNil())

		By("fetching corresponding secret")
		secret := &v1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, secret)
		}, timeout, interval).Should(Succeed())
		Expect(secret.Data).To(HaveKey("public-key"))

		By("validating status")
		gotPubKey := *wg.Status.PublicKey
		wantPubKey := string(secret.Data["public-key"])
		Expect(gotPubKey).To(Equal(wantPubKey))

	})

	It("should set endpoint to svc.clusterIp by default", func() {
		By("applying CRDs")
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{}, v1alpha1.WireguardStatus{})
		key := types.NamespacedName{
			Namespace: wg.GetNamespace(),
			Name:      wg.GetName(),
		}
		Eventually(func() error {
			return wgDsl.Apply(ctx, &wg)
		}, timeout, interval).Should(Succeed())

		By("refetching CRD from cluster")
		Eventually(func() error {
			return k8sClient.Get(ctx, key, &wg)
		}, timeout, interval).Should(Succeed())
		Expect(wg.Status.PublicKey).ToNot(BeNil())

		By("fetching corresponding service")
		service := &v1.Service{}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, service)
		}, timeout, interval).Should(Succeed())

		By("validating status")
		wantEndpoint := fmt.Sprintf("%s:%d", service.Spec.ClusterIP, wireguardPort)
		Expect(*wg.Status.Endpoint).To(Equal(wantEndpoint))
	})

	It("should set endpoint to .spec.endpoint if explicitly defined", func() {
		By("applying CRDs")
		ep := "127.0.0.1"
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
			EndpointAddress: toPtr(ep),
		}, v1alpha1.WireguardStatus{})
		Eventually(func() error {
			return wgDsl.Apply(ctx, &wg)
		}, timeout, interval).Should(Succeed())

		By("refetching CRD from cluster")
		key := types.NamespacedName{
			Namespace: wg.GetNamespace(),
			Name:      wg.GetName(),
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, &wg)
		}, timeout, interval).Should(Succeed())
		Expect(wg.Status.Endpoint).ToNot(BeNil())

		By("validating status")
		wantEndpoint := fmt.Sprintf("%s:%d", ep, wireguardPort)
		Expect(*wg.Status.Endpoint).To(Equal(wantEndpoint))
	})
})
