package factory

import (
	"fmt"
	"testing"

	"github.com/poy/onpar"
	"github.com/stretchr/testify/assert"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
	"github.com/cornbuddy/wireguard-operator/src/test/dsl"
)

func TestWireguardLabels(t *testing.T) {
	t.Parallel()

	testCases := []map[string]string{{
		"kek": "lel",
	}, {
		"app.kubernetes.io/managed-by": "kek",
	}}

	for _, labels := range testCases {
		wg := dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			Labels: labels,
		}, v1alpha1.WireguardStatus{})
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers:     v1alpha1.WireguardPeerList{},
		}

		got := fact.Labels()
		for key, value := range labels {
			assert.Contains(t, got, key)
			assert.Equal(t, value, got[key])
		}
	}
}

func TestWireguardResourcesShouldHaveProperDecorations(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	secret, err := defaultWgFact.Secret("kekeke", "kekeke")
	assert.Nil(t, err)

	configMap, err := defaultWgFact.ConfigMap()
	assert.Nil(t, err)

	service, err := defaultWgFact.Service()
	assert.Nil(t, err)

	deploy, err := defaultWgFact.Deployment("kekeke")
	assert.Nil(t, err)

	defaultLabels := map[string]string{
		"app.kubernetes.io/managed-by": "wireguard-operator",
	}

	o.Spec("Pods", func(t *testing.T) {
		assert.Equal(t, defaultLabels, deploy.Spec.Template.Labels)
	})

	type testCase struct {
		Name     string
		Resource metav1.Object
	}

	defaults := onpar.TableSpec(o, func(t *testing.T, tc testCase) {
		assert.Equal(t, defaultLabels, tc.Resource.GetLabels())
		shouldHaveProperAnnotations(t, tc.Resource)
	})

	testCases := []testCase{{
		Name:     "Service",
		Resource: secret,
	}, {
		Name:     "Deployment",
		Resource: deploy,
	}, {
		Name:     "ConfigMap",
		Resource: configMap,
	}, {
		Name:     "Service",
		Resource: service,
	}}

	for _, tc := range testCases {
		defaults.Entry(tc.Name, tc)
	}
}

func TestWireguardExtractEndpoint(t *testing.T) {
	t.Parallel()

	hostname := "localhost"
	clusterIp := "172.168.14.88"
	clusterIpSvc := corev1.Service{
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterIp,
		},
	}

	o := onpar.New(t)
	defer o.Run()

	o.Spec("should return cluster ip by default", func(t *testing.T) {
		wantEp := fmt.Sprintf("%s:%d", clusterIp, wireguardPort)
		gotEp, err := defaultWgFact.ExtractEndpoint(clusterIpSvc)
		assert.Nil(t, err)
		assert.Equal(t, wantEp, *gotEp)
	})

	o.Spec("should fail when public ip is not set", func(t *testing.T) {
		wg := dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			ServiceType: corev1.ServiceTypeLoadBalancer,
		}, v1alpha1.WireguardStatus{})
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers:     v1alpha1.WireguardPeerList{},
		}
		svc := corev1.Service{
			Spec: corev1.ServiceSpec{
				ClusterIP: clusterIp,
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{},
				},
			},
		}
		_, err := fact.ExtractEndpoint(svc)
		assert.Equal(t, ErrEndpointNotSet, err)
	})

	o.Spec("should return error when service type is NodePort", func(t *testing.T) {
		wg := dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			ServiceType: corev1.ServiceTypeNodePort,
		}, v1alpha1.WireguardStatus{})
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers:     v1alpha1.WireguardPeerList{},
		}
		_, err := fact.ExtractEndpoint(clusterIpSvc)
		assert.Equal(t, ErrUnsupportedServiceType, err)
	})

	type table struct {
		msg  string
		spec v1alpha1.WireguardSpec
		want string
	}

	spec := onpar.TableSpec(o, func(t *testing.T, tab table) {
		wg := dsl.GenerateWireguard(tab.spec, v1alpha1.WireguardStatus{})
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers:     v1alpha1.WireguardPeerList{},
		}

		// I don't care about service when .spec.enpointAdrress is set
		got, err := fact.ExtractEndpoint(clusterIpSvc)
		assert.Nil(t, err)
		assert.Equal(t, tab.want, *got)
	})

	endpointCases := []table{{
		msg: "should contain default wireguard port if .spec.endpointAddress does not contain port",
		spec: v1alpha1.WireguardSpec{
			EndpointAddress: toPtr("example.com"),
		},
		want: "example.com:51820",
	}, {
		msg: "should contain not default wireguard port if .spec.endpointAddress contains port",
		spec: v1alpha1.WireguardSpec{
			EndpointAddress: toPtr("example.com:1488"),
		},
		want: "example.com:1488",
	}, {
		msg: "should contain default wireguard port if .spec.endpointAddress contains default wireguard port",
		spec: v1alpha1.WireguardSpec{
			EndpointAddress: toPtr("example.com:51820"),
		},
		want: "example.com:51820",
	}, {
		msg: "should return hostname when serviceType == LoadBalancer",
		spec: v1alpha1.WireguardSpec{
			ServiceType: corev1.ServiceTypeLoadBalancer,
		},
		want: fmt.Sprintf("%s:%d", hostname, wireguardPort),
	}}

	for _, tab := range endpointCases {
		spec.Entry(tab.msg, tab)
	}
}

func TestWireguardConfigMap(t *testing.T) {
	t.Parallel()

	configMap, err := defaultWgFact.ConfigMap()
	assert.Nil(t, err)
	assert.NotNil(t, configMap)

	data := configMap.Data
	assert.Len(t, data, 1)
	assert.Contains(t, data, "entrypoint.sh")
	assert.NotEmpty(t, data["entrypoint.sh"])

	ep := data["entrypoint.sh"]
	assert.Contains(t, ep, "set -e")
	assert.Contains(t, ep, "wg-quick up")
	assert.Contains(t, ep, "wg-quick down")

}

func TestWireguardService(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	o.Spec("should work by default", func(t *testing.T) {
		svc, err := defaultWgFact.Service()
		assert.Nil(t, err)
		assert.NotEmpty(t, svc)
		assert.Empty(t, svc.Spec.ExternalTrafficPolicy)
		assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	})

	o.Spec("should have proper traffic policy when load balancer", func(t *testing.T) {
		wg := dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			ServiceType: corev1.ServiceTypeLoadBalancer,
		}, v1alpha1.WireguardStatus{})
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers: v1alpha1.WireguardPeerList{
				Items: []v1alpha1.WireguardPeer{defaultPeer},
			},
		}
		svc, err := fact.Service()
		assert.Nil(t, err)
		assert.Equal(t, corev1.ServiceTypeLoadBalancer, svc.Spec.Type)

		etp := svc.Spec.ExternalTrafficPolicy
		assert.Equal(t, corev1.ServiceExternalTrafficPolicyLocal, etp)
	})

	type testCase struct {
		description           string
		wireguard             v1alpha1.Wireguard
		serviceAnnotationsLen int
	}

	testCases := []testCase{{
		description: "no extra annotations",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		),
		serviceAnnotationsLen: 1,
	}, {
		description: "extra annotation",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{
				ServiceAnnotations: map[string]string{
					"top": "lel",
				},
			},
			v1alpha1.WireguardStatus{},
		),
		serviceAnnotationsLen: 2,
	}}

	spec := onpar.TableSpec(o, func(t *testing.T, test testCase) {
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: test.wireguard,
			Peers:     v1alpha1.WireguardPeerList{},
		}
		svc, err := fact.Service()
		assert.Nil(t, err)

		actual := svc.ObjectMeta.Annotations
		assert.Len(t, actual, test.serviceAnnotationsLen)

		for key, value := range test.wireguard.Spec.ServiceAnnotations {
			assert.Contains(t, actual, key)
			assert.Equal(t, actual[key], value)
		}
	})

	for _, tc := range testCases {
		spec.Entry(tc.description, tc)
	}
}

func TestWireguardSecret(t *testing.T) {
	t.Parallel()

	o := onpar.New(t)
	defer o.Run()

	key, err := wgtypes.GeneratePrivateKey()
	assert.Nil(t, err)

	wantPrivKey := key.String()
	wantPubKey := key.PublicKey().String()

	o.Spec("should generate proper secret by default", func(t *testing.T) {
		secret, err := defaultWgFact.Secret(wantPubKey, wantPrivKey)
		assert.Nil(t, err)

		data := secret.Data
		keys := []string{"public-key", "private-key", "config"}
		for _, key := range keys {
			assert.Contains(t, data, key)
		}

		config := string(data["config"])
		assert.NotEmpty(t, config)
		assert.Contains(t, config, wantPrivKey)
		assert.NotContains(t, config, "PersistentKeepalive")

		wg := defaultWireguard
		lines := []string{
			fmt.Sprintf("Address = %s", wg.Spec.Address),
			fmt.Sprintf("PrivateKey = %s", wantPrivKey),
			fmt.Sprintf("ListenPort = %d", wireguardPort),
			fmt.Sprintf("[Peer]\n# friendly_name = %s", defaultPeer.GetName()),
			"PostUp = iptables --append FORWARD --in-interface %i --jump ACCEPT",
			"PostUp = iptables --append FORWARD --out-interface %i --jump ACCEPT",
			"PostUp = iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE",
			"SaveConfig = false",
		}

		for _, line := range lines {
			assert.Contains(t, config, line)
		}

		gotPrivKey := string(secret.Data["private-key"])
		gotPubKey := string(secret.Data["public-key"])
		assert.Equal(t, wantPrivKey, gotPrivKey)
		assert.Equal(t, wantPubKey, gotPubKey)
	})

	o.Spec("should skip peer if it's not ready", func(t *testing.T) {
		wg := dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		peerAddr := v1alpha1.Address("192.168.420.228/42")
		notReadyPeer := dsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{
				WireguardRef: wg.GetName(),
				Address:      peerAddr,
			}, v1alpha1.WireguardPeerStatus{},
		)
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers: v1alpha1.WireguardPeerList{
				Items: []v1alpha1.WireguardPeer{notReadyPeer},
			},
		}
		secret, err := fact.Secret(wantPubKey, wantPrivKey)
		assert.Nil(t, err)
		assert.NotNil(t, secret)
		assert.Contains(t, secret.Data, "config")

		config := string(secret.Data["config"])
		assert.NotContains(t, config, peerAddr,
			"should skip peers with empty public key in status")
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
		description: "defaults",
		wireguard: dsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		),
	}, {
		description: "external dns",
		wireguard: dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			DNS: "1.1.1.1",
		}, v1alpha1.WireguardStatus{}),
	}, {
		description: "internal dns",
		wireguard: dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			DNS: "internal.dns.svc",
		}, v1alpha1.WireguardStatus{}),
	}, {
		description: "sidecar",
		wireguard: dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			Sidecars: []corev1.Container{sidecar},
		}, v1alpha1.WireguardStatus{}),
	}}

	hashStub := "kekeke"

	spec := onpar.TableSpec(o, func(t *testing.T, test testCase) {
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: test.wireguard,
			Peers:     v1alpha1.WireguardPeerList{},
		}

		deploy, err := fact.Deployment(hashStub)
		assert.Nil(t, err)

		podSpec := deploy.Spec.Template.Spec
		epMode := podSpec.Volumes[1].ConfigMap.Items[0].Mode
		assert.Equal(t, toPtr[int32](0755), epMode)
		assert.Len(t, podSpec.Volumes, 2)
		assert.Nil(t, podSpec.DNSConfig)

		containers := podSpec.Containers
		assert.Len(t, containers, len(test.wireguard.Spec.Sidecars)+1)

		wgCont := containers[0]
		assert.Equal(t, "wireguard", wgCont.Name)
		assert.Len(t, wgCont.Command, 1)
		assert.Contains(t, wgCont.Command, "/opt/bin/entrypoint.sh",
			"should have proper entrypoint set")

		wantContext := &corev1.SecurityContext{
			Privileged: toPtr(true),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN",
					"SYS_MODULE",
				},
			},
		}
		wgContainer := deploy.Spec.Template.Spec.Containers[0]
		assert.EqualValues(t, wantContext, wgContainer.SecurityContext)

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
		assert.EqualValues(t, wantSysctls, gotSysctls)
	})

	for _, tc := range testCases {
		spec.Entry(tc.description, tc)
	}
}
