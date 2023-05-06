package controllers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vpnv1alpha1 "github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	//+kubebuilder:scaffold:imports
)

const (
	timeout  = 1 * time.Second
	interval = 200 * time.Millisecond
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
)

type Reconciler interface {
	Reconcile(context.Context, ctrl.Request) (ctrl.Result, error)
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = vpnv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// Validates reconcilation of custom resource. Creates custom resource as a side
// effect
func validateReconcile(object client.Object, reconciler Reconciler) {
	key := types.NamespacedName{
		Name:      object.GetName(),
		Namespace: object.GetNamespace(),
	}

	Eventually(func(g Gomega) {
		By("Creating the custom resource")
		g.Expect(k8sClient.Create(context.TODO(), object)).To(Succeed())

		By("Checking if the custom resource was successfully created")
		g.Expect(k8sClient.Get(context.TODO(), key, object)).To(Succeed())

		By("Reconciling the custom resource created")
		g.Expect(reconcileCustomResource(key, reconciler)).To(Succeed())
	}, timeout, interval).Should(Succeed())
}

// Performs full reconcildation loop for wireguard
func reconcileCustomResource(key types.NamespacedName, reconciler Reconciler) error {
	// Reconcile resource multiple times to ensure that all resources are
	// created
	const reconcilationLoops = 5
	for i := 0; i < reconcilationLoops; i++ {
		req := reconcile.Request{
			NamespacedName: key,
		}
		if _, err := reconciler.Reconcile(context.TODO(), req); err != nil {
			return err
		}
	}
	return nil
}
