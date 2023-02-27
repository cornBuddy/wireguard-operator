package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
)

var _ = Describe("Wireguard controller", func() {
	Context("Wireguard controller test", func() {

		const WireguardName = "test-wireguard"

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      WireguardName,
				Namespace: WireguardName,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: WireguardName, Namespace: WireguardName}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting the Image ENV VAR which stores the Operand image")
			err = os.Setenv("WIREGUARD_IMAGE", "example.com/image:test")
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)

			By("Removing the Image ENV VAR which stores the Operand image")
			_ = os.Unsetenv("WIREGUARD_IMAGE")
		})

		It("should successfully reconcile a custom resource for Wireguard", func() {
			By("Creating the custom resource for the Kind Wireguard")
			wireguard := &vpnv1alpha1.Wireguard{}
			err := k8sClient.Get(ctx, typeNamespaceName, wireguard)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				wireguard := &vpnv1alpha1.Wireguard{
					ObjectMeta: metav1.ObjectMeta{
						Name:      WireguardName,
						Namespace: namespace.Name,
					},
					Spec: vpnv1alpha1.WireguardSpec{
						Replicas:      1,
						ContainerPort: 51820,
					},
				}

				err = k8sClient.Create(ctx, wireguard)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &vpnv1alpha1.Wireguard{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Reconciling the custom resource created")
			wireguardReconciler := &WireguardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = wireguardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				found := &appsv1.Deployment{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking the latest Status Condition added to the Wireguard instance")
			Eventually(func() error {
				if wireguard.Status.Conditions != nil && len(wireguard.Status.Conditions) != 0 {
					latestStatusCondition := wireguard.Status.Conditions[len(wireguard.Status.Conditions)-1]
					msg := fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", wireguard.Name, wireguard.Spec.Replicas)
					expectedLatestStatusCondition := metav1.Condition{
						Type:   typeAvailableWireguard,
						Status: metav1.ConditionTrue, Reason: "Reconciling",
						Message: msg,
					}
					if latestStatusCondition != expectedLatestStatusCondition {
						return fmt.Errorf("The latest status condition added to the wireguard instance is not as expected")
					}
				}
				return nil
			}, time.Minute, time.Second).Should(Succeed())
		})
	})
})
