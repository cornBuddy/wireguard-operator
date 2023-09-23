package factory

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"k8s.io/api/core/v1"

	"github.com/ahova-vpn/wireguard-operator/api/v1alpha1"
	"github.com/ahova-vpn/wireguard-operator/private/testdsl"
)

var _ = Describe("Wireguard#ExtractEndpoint", func() {
	clusterIp := "172.168.14.88"
	wantEp := fmt.Sprintf("%s:%d", clusterIp, wireguardPort)
	svc := v1.Service{
		Spec: v1.ServiceSpec{
			ClusterIP: clusterIp,
		},
	}

	It("should return error when service type is NodePort", func() {
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
			ServiceType: v1.ServiceTypeNodePort,
		}, v1alpha1.WireguardStatus{})
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers:     v1alpha1.WireguardPeerList{},
		}
		_, err := fact.ExtractEndpoint(svc)
		Expect(err).ToNot(BeNil())
	})

	It("should return cluster ip by default", func() {
		gotEp, err := wireguardFactory.ExtractEndpoint(svc)
		Expect(err).To(BeNil())
		Expect(*gotEp).To(Equal(wantEp))
	})

	It("should return cluster ip when serviceType == LoadBalancer", func() {
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
			ServiceType: v1.ServiceTypeLoadBalancer,
		}, v1alpha1.WireguardStatus{})
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers:     v1alpha1.WireguardPeerList{},
		}
		gotEp, err := fact.ExtractEndpoint(svc)
		Expect(err).To(BeNil())
		Expect(*gotEp).To(Equal(wantEp))
	})
})

var _ = Describe("Wireguard#ConfigMap", func() {
	It("should return valid configmap by default", func() {
		configMap, err := wireguardFactory.ConfigMap()
		Expect(err).To(BeNil())
		Expect(configMap).ToNot(BeNil())
		shouldHaveProperDecorations(configMap)

		data := configMap.Data
		Expect(data).To(HaveKey("unbound.conf"))
		Expect(data["unbound.conf"]).ToNot(BeEmpty())

	})
})

var _ = Describe("Wireguard#Service", func() {
	It("should return valid service by default", func() {
		service, err := wireguardFactory.Service()
		Expect(err).To(BeNil())
		Expect(service).ToNot(BeNil())
		shouldHaveProperDecorations(service)
	})
})

var _ = Describe("Wireguard#Secret", func() {
	It("should return valid secret by default", func() {
		key, err := wgtypes.GeneratePrivateKey()
		Expect(err).To(BeNil())

		wantPrivKey := key.String()
		wantPubKey := key.PublicKey().String()
		secret, err := wireguardFactory.Secret(wantPubKey, wantPrivKey, "kek")
		Expect(err).To(BeNil())
		shouldHaveProperDecorations(secret)

		Expect(secret.Data).To(HaveKey("public-key"))
		Expect(secret.Data).To(HaveKey("private-key"))
		Expect(secret.Data).To(HaveKey("config"))

		gotPrivKey := string(secret.Data["private-key"])
		gotPubKey := string(secret.Data["public-key"])
		Expect(gotPrivKey).To(Equal(wantPrivKey))
		Expect(gotPubKey).To(Equal(wantPubKey))

		config := string(secret.Data["config"])
		Expect(config).ToNot(BeEmpty())
		Expect(config).To(ContainSubstring(wantPrivKey))
	})

	It("should skip peers with empty public key in status", func() {
		key, err := wgtypes.GeneratePrivateKey()
		Expect(err).To(BeNil())

		wg := testdsl.GenerateWireguard(
			v1alpha1.WireguardSpec{},
			v1alpha1.WireguardStatus{},
		)
		peerAddr := "192.168.420.228"
		peer := testdsl.GeneratePeer(
			v1alpha1.WireguardPeerSpec{
				WireguardRef: wg.GetName(),
				Address:      peerAddr,
			}, v1alpha1.WireguardPeerStatus{},
		)
		fact := Wireguard{
			Scheme:    scheme,
			Wireguard: wg,
			Peers: v1alpha1.WireguardPeerList{
				Items: []v1alpha1.WireguardPeer{peer},
			},
		}

		wantPrivKey := key.String()
		wantPubKey := key.PublicKey().String()
		secret, err := fact.Secret(wantPubKey, wantPrivKey, "127.0.0.1")
		Expect(err).To(BeNil())
		Expect(secret).ToNot(BeNil())
		Expect(secret.Data).To(HaveKey("config"))

		config := string(secret.Data["config"])
		Expect(config).ToNot(ContainSubstring(peerAddr))
	})
})

var _ = Describe("Wireguard#Deployment", func() {
	hashStub := "kekeke"

	It("should have proper decorations", func() {
		deploy, err := wireguardFactory.Deployment(hashStub)
		Expect(err).To(BeNil())
		shouldHaveProperDecorations(deploy)
	})

	It("should not fail with default wireguard instance", func() {
		deploy, err := wireguardFactory.Deployment(hashStub)
		Expect(err).To(BeNil())

		podSpec := deploy.Spec.Template.Spec
		Expect(podSpec.Containers).To(HaveLen(2))

		dnsConfig := &v1.PodDNSConfig{
			Nameservers: []string{"127.0.0.1"},
		}
		Expect(podSpec.DNSConfig).To(BeEquivalentTo(dnsConfig))
		Expect(podSpec.DNSPolicy).To(Equal(v1.DNSNone))
	})

	It("should deploy dns sidecar when wireguard.spec.dns.deployServer == true", func() {
		deploy, err := wireguardFactory.Deployment(hashStub)
		Expect(err).To(BeNil())

		podSpec := deploy.Spec.Template.Spec
		dnsConfig := &v1.PodDNSConfig{
			Nameservers: []string{"127.0.0.1"},
		}
		Expect(podSpec.DNSConfig).To(BeEquivalentTo(dnsConfig))
		Expect(podSpec.DNSPolicy).To(Equal(v1.DNSNone))

		containers := podSpec.Containers
		Expect(containers).To(HaveLen(2))
	})

	It("should not deploy dns sidecar when wireguard.spec.dns.deployServer == false", func() {
		wg := testdsl.GenerateWireguard(v1alpha1.WireguardSpec{
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
		Expect(err).To(BeNil())

		podSpec := deploy.Spec.Template.Spec
		Expect(podSpec.DNSConfig).To(BeNil())
		Expect(podSpec.DNSPolicy).To(Equal(v1.DNSClusterFirst))

		containers := podSpec.Containers
		Expect(containers).To(HaveLen(1))
	})
})
