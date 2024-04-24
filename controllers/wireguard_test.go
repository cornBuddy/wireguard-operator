package controllers

import (
	"fmt"
	"testing"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/test/dsl"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/poy/onpar"
	"github.com/stretchr/testify/assert"
)

func TestWireguardSecret(t *testing.T) {
	t.Parallel()

	type testContext struct {
		t         *testing.T
		key       types.NamespacedName
		wireguard *v1alpha1.Wireguard
		secret    *corev1.Secret
	}

	o := onpar.BeforeEach(onpar.New(t), func(t *testing.T) testContext {
		wg := dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		err := wgDsl.Apply(ctx, &wg)
		assert.Nil(t, err)

		key := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, key, secret)
		assert.Nil(t, err)

		return testContext{
			t:         t,
			key:       key,
			wireguard: &wg,
			secret:    secret,
		}
	})
	defer o.Run()

	o.AfterEach(func(tc testContext) {
		err := k8sClient.Delete(ctx, tc.wireguard)
		assert.Nil(t, err)
	})

	type testCase struct {
		description string
		peer        v1alpha1.WireguardPeer
	}

	testCases := []testCase{{
		description: "when peer is default",
		peer: dsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{},
			v1alpha1.WireguardPeerStatus{},
		),
	}}

	spec := onpar.TableSpec(o, func(testCtx testContext, tc testCase) {
		t := testCtx.t
		wg := testCtx.wireguard

		peer := tc.peer
		peer.Spec.WireguardRef = wg.GetName()
		err := peerDsl.Apply(ctx, &peer)
		assert.Nil(t, err, "apply peer into cluster")

		err = wgDsl.Reconcile(wg)
		assert.Nil(t, err, "fetch new peer from cluster")

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, testCtx.key, secret)
		assert.Nil(t, err, "fetch updated secret")

		peerKey := types.NamespacedName{
			Name:      peer.GetName(),
			Namespace: peer.GetNamespace(),
		}
		peerSecret := &corev1.Secret{}
		err = k8sClient.Get(ctx, peerKey, peerSecret)
		assert.Nil(t, err, "peer should reconcile into secret")
		assert.Contains(t, peerSecret.Data, "public-key")

		config := string(secret.Data["config"])
		assert.NotContains(t, config, "Endpoint =")

		lines := []string{
			"PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 192.168.0.0/16 --jump DROP",
			"PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 172.16.0.0/12 --jump DROP",
			"PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 10.0.0.0/8 --jump DROP",
			"PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 169.254.169.254/32 --jump DROP",
			"[Peer]",
			"SaveConfig = false",
			fmt.Sprintf("AllowedIPs = %s", peer.Spec.Address),
			fmt.Sprintf("PublicKey = %s", peerSecret.Data["public-key"]),
		}
		for _, line := range lines {
			assert.Contains(t, config, line, "has proper config")
		}

	})

	for _, tc := range testCases {
		spec.Entry(tc.description, tc)
	}

	o.Spec("generated keys should be valid", func(tc testContext) {
		t := tc.t
		data := tc.secret.Data
		keys := []string{"public-key", "private-key"}
		for _, key := range keys {
			assert.Contains(t, data, key)
			assert.Len(t, data[key], 44)
		}

		privKey := string(data["private-key"])
		wgKey, err := wgtypes.ParseKey(privKey)
		assert.Nil(t, err)

		gotPubKey := string(data["public-key"])
		wantPubKey := wgKey.PublicKey().String()
		assert.Equal(t, wantPubKey, gotPubKey)
	})

	o.Spec("should not regenerate keys between reconcilations", func(tc testContext) {
		t := tc.t
		wg := tc.wireguard

		// simulate reconcilation loop
		err := wgDsl.Reconcile(wg)
		assert.Nil(t, err)

		gotSecret := &corev1.Secret{}
		err = k8sClient.Get(ctx, tc.key, gotSecret)
		assert.Nil(t, err)

		keys := []string{"public-key", "private-key"}
		for _, key := range keys {
			wantKey := tc.secret.Data[key]
			gotKey := gotSecret.Data[key]
			assert.Equal(t, wantKey, gotKey)
		}
	})
}

func TestWireguardDeployment(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	type testCase struct {
		description string
		wireguard   v1alpha1.Wireguard
	}

	testCases := []testCase{{
		description: "with default config",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		),
	}, {
		description: "with external dns spec",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{
				DNS: &v1alpha1.DNS{
					DeployServer: false,
					Address:      "127.0.0.1",
				}},
			v1alpha1.WireguardStatus{},
		),
	}}

	spec := onpar.TableSpec(o, func(t *testing.T, test testCase) {
		wg := test.wireguard
		err := wgDsl.Apply(ctx, &wg)
		assert.Nil(t, err)

		key := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		deploy := &appsv1.Deployment{}
		err = k8sClient.Get(ctx, key, deploy)
		assert.Nil(t, err)

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, key, secret)
		assert.Nil(t, err)
		assert.Contains(t, secret.Data, "config")

		config := secret.Data["config"]
		podAnnotations := deploy.Spec.Template.Annotations
		assert.Contains(t, podAnnotations, "vpn.ahova.com/config-hash")

		wantHash := makeHash(config)
		gotHash := podAnnotations["vpn.ahova.com/config-hash"]
		assert.Equal(t, wantHash, gotHash)
	})

	for _, tc := range testCases {
		spec.Entry(tc.description, tc)
	}

}

func TestWireguardService(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	o.Spec("service type can be updated", func(t *testing.T) {
		status := v1alpha1.WireguardStatus{}
		spec1 := v1alpha1.WireguardSpec{
			ServiceType: corev1.ServiceTypeClusterIP,
		}
		spec2 := v1alpha1.WireguardSpec{
			ServiceType: corev1.ServiceTypeLoadBalancer,
		}

		wg := dsl.GenerateWireguard(spec1, status)
		err := wgDsl.Apply(ctx, &wg)
		assert.Nil(t, err)

		key := types.NamespacedName{
			Name:      wg.GetName(),
			Namespace: wg.GetNamespace(),
		}
		svc1 := &corev1.Service{}
		err = k8sClient.Get(ctx, key, svc1)
		assert.Nil(t, err)

		err = k8sClient.Get(ctx, key, &wg)
		assert.Nil(t, err)

		wg.Spec = spec2
		err = k8sClient.Update(ctx, &wg)
		assert.Nil(t, err)

		err = wgDsl.Reconcile(&wg)
		assert.Nil(t, err)

		svc2 := &corev1.Service{}
		err = k8sClient.Get(ctx, key, svc2)
		assert.Nil(t, err)
		assert.Equal(t, spec1.ServiceType, svc1.Spec.Type)
		assert.Equal(t, spec2.ServiceType, svc2.Spec.Type)
	})
}

func TestWireguardStatus(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	type testCase struct {
		description     string
		wireguard       v1alpha1.Wireguard
		extractEndpoint endpointExtractor
	}

	testCases := []testCase{{
		description: "should set to clusterIp by default",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		),
		extractEndpoint: extractClusterIp,
	}, {
		description: "should set endpoint to .spec.endpoint if defined",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{
				EndpointAddress: toPtr("127.0.0.1"),
			},
			v1alpha1.WireguardStatus{},
		),
		extractEndpoint: extractWireguardEndpoint,
	}}

	spec := onpar.TableSpec(o, func(t *testing.T, test testCase) {
		wg := test.wireguard
		key := types.NamespacedName{
			Namespace: wg.GetNamespace(),
			Name:      wg.GetName(),
		}
		err := wgDsl.Apply(ctx, &wg)
		assert.Nil(t, err)

		err = k8sClient.Get(ctx, key, &wg)
		assert.Nil(t, err)
		assert.NotNil(t, wg.Status.PublicKey)
		assert.NotNil(t, wg.Status.Endpoint)

		service := &corev1.Service{}
		assert.Eventually(t, func() bool {
			return k8sClient.Get(ctx, key, service) == nil
		}, timeout, tick)

		ep := test.extractEndpoint(wg, *service)
		wantEndpoint := fmt.Sprintf("%s:%d", ep, wireguardPort)
		assert.Equal(t, wantEndpoint, *wg.Status.Endpoint)
	})

	for _, tc := range testCases {
		spec.Entry(tc.description, tc)
	}

	o.Spec("should set public key same as in the secret", func(t *testing.T) {
		wg := dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		key := types.NamespacedName{
			Namespace: wg.GetNamespace(),
			Name:      wg.GetName(),
		}
		err := wgDsl.Apply(ctx, &wg)
		assert.Nil(t, err)

		err = k8sClient.Get(ctx, key, &wg)
		assert.Nil(t, err)
		assert.NotNil(t, wg.Status.PublicKey)

		secret := &corev1.Secret{}
		assert.Eventually(t, func() bool {
			return k8sClient.Get(ctx, key, secret) == nil
		}, timeout, tick)
		assert.Contains(t, secret.Data, "public-key")

		gotPubKey := *wg.Status.PublicKey
		wantPubKey := string(secret.Data["public-key"])
		assert.Equal(t, wantPubKey, gotPubKey)
	})
}

func TestWireguardConfigMap(t *testing.T) {
	t.Parallel()

	type testCase struct {
		description string
		wireguard   v1alpha1.Wireguard
	}

	sidecar := corev1.Container{
		Name:  "wireguard-exporter",
		Image: "docker.io/mindflavor/prometheus-wireguard-exporter:3.6.6",
		Args: []string{
			"--verbose", "true",
			"--extract_names_config_files", "/config/wg0.conf",
		},
		VolumeMounts: []corev1.VolumeMount{{
			Name:      "config",
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

	testCases := []testCase{{
		description: "default configuration",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		),
	}, {
		description: "internal dns configuration",
		wireguard: dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			DNS: &v1alpha1.DNS{
				DeployServer: false,
				Address:      "10.96.0.1",
			},
		}, v1alpha1.WireguardStatus{}),
	}, {
		description: "sidecar configuration",
		wireguard: dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			Sidecars: []corev1.Container{sidecar},
		}, v1alpha1.WireguardStatus{}),
	}}

	o := onpar.New(t)
	defer o.Run()

	spec := onpar.TableSpec(o, func(t *testing.T, test testCase) {
		err := wgDsl.Apply(ctx, &test.wireguard)
		assert.Nil(t, err)

		cm := &corev1.ConfigMap{}
		key := types.NamespacedName{
			Name:      test.wireguard.GetName(),
			Namespace: test.wireguard.GetNamespace(),
		}
		err = k8sClient.Get(ctx, key, cm)
		assert.Nil(t, err, "should create config map")
	})

	for _, tc := range testCases {
		spec.Entry(tc.description, tc)
	}
}
