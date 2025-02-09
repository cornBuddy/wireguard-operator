package factory

import (
	"fmt"
	"testing"

	"github.com/cornbuddy/wireguard-operator/src/api/v1alpha1"
	"github.com/cornbuddy/wireguard-operator/src/test/dsl"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/poy/onpar"
	"github.com/stretchr/testify/assert"
)

func TestWireguardResourcesShouldHaveProperDecorations(t *testing.T) {
	t.Parallel()

	secret, err := defaultWgFact.Secret("kekeke", "kekeke")
	assert.Nil(t, err)

	deploy, err := defaultWgFact.Deployment("kekeke")
	assert.Nil(t, err)

	configMap, err := defaultWgFact.ConfigMap()
	assert.Nil(t, err)

	service, err := defaultWgFact.Service()
	assert.Nil(t, err)

	var resources []metav1.Object = []metav1.Object{
		secret, deploy, configMap, service,
	}
	for _, resource := range resources {
		shouldHaveProperDecorations(t, resource)
	}

}

func TestWireguardExtractEndpoint(t *testing.T) {
	t.Parallel()

	clusterIp := "172.168.14.88"
	clusterIpSvc := corev1.Service{
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterIp,
		},
	}
	hostname := "localhost"
	loadBalancerSvc := corev1.Service{
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterIp,
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{
					Hostname: hostname,
				}},
			},
		},
	}

	wantEp := fmt.Sprintf("%s:%d", clusterIp, wireguardPort)
	gotEp, err := defaultWgFact.ExtractEndpoint(clusterIpSvc)
	assert.Nil(t, err)
	assert.Equal(t, wantEp, *gotEp, "should return cluster ip by default")

	wg := dsl.GenerateWireguard(v1alpha1.WireguardSpec{
		ServiceType: corev1.ServiceTypeNodePort,
	}, v1alpha1.WireguardStatus{})
	fact := Wireguard{
		Scheme:    scheme,
		Wireguard: wg,
		Peers:     v1alpha1.WireguardPeerList{},
	}
	_, err = fact.ExtractEndpoint(clusterIpSvc)
	assert.Error(t, err,
		"should return error when service type is NodePort")

	wg = dsl.GenerateWireguard(v1alpha1.WireguardSpec{
		ServiceType: corev1.ServiceTypeLoadBalancer,
	}, v1alpha1.WireguardStatus{})
	fact = Wireguard{
		Scheme:    scheme,
		Wireguard: wg,
		Peers:     v1alpha1.WireguardPeerList{},
	}
	wantEp = fmt.Sprintf("%s:%d", hostname, wireguardPort)
	gotEp, err = fact.ExtractEndpoint(loadBalancerSvc)
	assert.Nil(t, err)
	assert.Equal(t, wantEp, *gotEp,
		"should return hostname when serviceType == LoadBalancer")
}

func TestWireguardConfigMap(t *testing.T) {
	t.Parallel()

	configMap, err := defaultWgFact.ConfigMap()
	assert.Nil(t, err)
	assert.NotNil(t, configMap)

	data := configMap.Data
	assert.Contains(t, data, "unbound.conf")
	assert.Contains(t, data, "entrypoint.sh")
	assert.NotEmpty(t, data["unbound.conf"])
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
			// "PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 192.168.0.0/16 --jump DROP",
			// "PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 172.16.0.0/12 --jump DROP",
			// "PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 10.0.0.0/8 --jump DROP",
			// "PostUp = iptables --insert FORWARD --source 192.168.254.1/24 --destination 169.254.169.254/32 --jump DROP",
			"PostUp = iptables --append FORWARD --in-interface %i --jump ACCEPT",
			"PostUp = iptables --append FORWARD --out-interface %i --jump ACCEPT",
			"PostUp = iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE",
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
				Address:      "127.0.0.1",
			},
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

		wgCont := podSpec.Containers[0]
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

	o.Spec("default dns config", func(t *testing.T) {
		deploy, err := defaultWgFact.Deployment(hashStub)
		assert.Nil(t, err)

		podSpec := deploy.Spec.Template.Spec
		assert.Len(t, podSpec.Volumes, 3)
		assert.Len(t, podSpec.Containers, 2)

		wantDnsConfig := &corev1.PodDNSConfig{
			Nameservers: []string{"127.0.0.1"},
		}
		assert.EqualValues(t, wantDnsConfig, podSpec.DNSConfig)
		assert.Equal(t, corev1.DNSNone, podSpec.DNSPolicy,
			"should set proper DNS none policy by default")
	})

	o.Spec("internal dns configuration", func(t *testing.T) {
		wg := dsl.GenerateWireguard(v1alpha1.WireguardSpec{
			DNS: &v1alpha1.DNS{
				DeployServer: false,
				Address:      "127.0.0.1",
			},
		}, v1alpha1.WireguardStatus{})
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers:     v1alpha1.WireguardPeerList{},
		}
		deploy, err := fact.Deployment(hashStub)
		assert.Nil(t, err)

		podSpec := deploy.Spec.Template.Spec
		assert.Nil(t, podSpec.DNSConfig)
		assert.Equal(t, corev1.DNSClusterFirst, podSpec.DNSPolicy)
		assert.Len(t, podSpec.Volumes, 2)
		assert.Len(t, podSpec.Containers, 1)

		containers := podSpec.Containers
		assert.Len(t, containers, 1)
	})

}
