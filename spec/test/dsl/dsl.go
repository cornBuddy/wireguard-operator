package dsl

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"text/template"
	"time"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sRuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	PeerServiceName = "peer"

	kubeDnsSample  = "../src/config/samples/kube-dns.yml"
	sidecarSample  = "../src/config/samples/sidecar.yml"
	wireguardImage = "linuxserver/wireguard:1.0.20210914"
)

var (
	samples      = []string{kubeDnsSample, sidecarSample}
	wireguardGvr = schema.GroupVersionResource{
		Group:    "vpn.ahova.com",
		Version:  "v1alpha1",
		Resource: "wireguards",
	}
	peerGvr = schema.GroupVersionResource{
		Group:    "vpn.ahova.com",
		Version:  "v1alpha1",
		Resource: "wireguardpeers",
	}
)

type Dsl struct {
	Client              *kubernetes.Clientset
	apiExtensionsClient *clientset.Clientset
	dynamicClient       *dynamic.DynamicClient
	ctx                 context.Context
	t                   *testing.T
}

type spec map[string]interface{}

func (dsl Dsl) CreatePeerWithSpec(namespace string, spec spec) (string, error) {
	name := randomString()
	obj := map[string]interface{}{
		"apiVersion": "vpn.ahova.com/v1alpha1",
		"kind":       "WireguardPeer",
		"metadata": map[string]string{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}
	unstrcd := &unstructured.Unstructured{
		Object: obj,
	}

	opts := metav1.CreateOptions{}
	dri := dsl.dynamicClient.Resource(peerGvr).Namespace(namespace)
	if _, err := dri.Create(dsl.ctx, unstrcd, opts); err != nil {
		return "", err
	}

	return name, nil

}

func (dsl Dsl) DeletePeer(name, namespace string) error {
	opts := metav1.DeleteOptions{}
	dri := dsl.dynamicClient.Resource(peerGvr).Namespace(namespace)
	if err := dri.Delete(dsl.ctx, name, opts); err != nil {
		return err
	}

	return nil
}

func (dsl Dsl) CreateWireguardWithSpec(namespace string, spec spec) (string, error) {
	if spec["serviceAnnotations"] == nil {
		spec["serviceAnnotations"] = map[string]interface{}{}
	}

	// annotations below are required for eks, and I don't want to copypaste
	// them in each procedure call
	defaultServiceAnnotations := map[string]interface{}{
		"service.beta.kubernetes.io/aws-load-balancer-type":                              "nlb",
		"service.beta.kubernetes.io/aws-load-balancer-backend-protocol":                  "udp",
		"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
		"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                  "10250",
		"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":              "tcp",
	}
	svcAnnotations := spec["serviceAnnotations"].(map[string]interface{})
	maps.Copy(svcAnnotations, defaultServiceAnnotations)
	spec["serviceAnnotations"] = svcAnnotations

	name := randomString()
	obj := map[string]interface{}{
		"apiVersion": "vpn.ahova.com/v1alpha1",
		"kind":       "Wireguard",
		"metadata": map[string]string{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}
	unstrcd := &unstructured.Unstructured{
		Object: obj,
	}

	opts := metav1.CreateOptions{}
	dri := dsl.dynamicClient.Resource(wireguardGvr).Namespace(namespace)
	if _, err := dri.Create(dsl.ctx, unstrcd, opts); err != nil {
		return "", err
	}

	return name, nil
}

func (dsl Dsl) DeleteWireguard(name, namespace string) error {
	opts := metav1.DeleteOptions{}
	dri := dsl.dynamicClient.Resource(wireguardGvr).Namespace(namespace)
	if err := dri.Delete(dsl.ctx, name, opts); err != nil {
		return err
	}

	return nil
}

func (dsl Dsl) StartPeerWithConfig(peerConfig string) (
	compose.ComposeStack, error) {

	configPath, err := dsl.makeTempConfig(peerConfig)
	if err != nil {
		return nil, err
	}

	composePath, err := dsl.makeTempComposeFile(configPath)
	if err != nil {
		return nil, err
	}

	peerCompose, err := compose.NewDockerCompose(composePath)
	if err != nil {
		return nil, err
	}

	waitForWg := wait.ForExec(
		[]string{"/bin/bash", "-c", "wg"},
	).
		WithExitCodeMatcher(func(code int) bool {
			return code == 0
		})
	waits := []wait.Strategy{
		wait.ForLog("resolvconf -a wg0 -m 0 -x").AsRegexp(),
		waitForWg,
	}
	stack := peerCompose.WaitForService(
		PeerServiceName,
		wait.ForAll(waits...).WithDeadline(3*time.Second),
	)
	if err := stack.Up(
		dsl.ctx,
		compose.Wait(true),
		compose.RemoveOrphans(true),
	); err != nil {
		return nil, err
	}

	return stack, nil
}

func (dsl Dsl) ApplySamples(namespace string) error {
	// https://gist.github.com/pytimer/0ad436972a073bb37b8b6b8b474520fc
	for _, sample := range samples {
		obj, gvk, err := dsl.readObjectFromFile(sample)
		if err != nil {
			return err
		}

		unstrcd, res, err := dsl.objectToUnstructured(obj, gvk)
		if err != nil {
			return err
		}

		opts := metav1.CreateOptions{}
		dri := dsl.dynamicClient.Resource(*res).Namespace(namespace)
		if _, err := dri.Create(dsl.ctx, unstrcd, opts); err != nil {
			return err
		}
	}

	return nil
}

func (dsl Dsl) DeleteSamples(namespace string) error {
	for _, sample := range samples {
		obj, gvk, err := dsl.readObjectFromFile(sample)
		if err != nil {
			return err
		}

		unstrcd, res, err := dsl.objectToUnstructured(obj, gvk)
		if err != nil {
			return err
		}

		opts := metav1.DeleteOptions{}
		name := unstrcd.GetName()
		dri := dsl.dynamicClient.Resource(*res).Namespace(namespace)
		if err := dri.Delete(dsl.ctx, name, opts); err != nil {
			return err
		}
	}

	return nil
}

func NewDsl(ctx context.Context, t *testing.T) (*Dsl, error) {
	apiExtClient, err := MakeApiExtensionsClient()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := makeDynamicClient()
	if err != nil {
		return nil, err
	}

	staticClient, err := makeStaticClient()
	if err != nil {
		return nil, err
	}

	return &Dsl{
		Client:              staticClient,
		apiExtensionsClient: apiExtClient,
		dynamicClient:       dynamicClient,
		ctx:                 ctx,
		t:                   t,
	}, nil
}

func MakeApiExtensionsClient() (*clientset.Clientset, error) {
	kubeConfig, err := makeKubeConfig()
	if err != nil {
		return nil, err
	}

	client, err := clientset.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// generate docker compose file for peer with configuration mounted into
// container
func (dsl Dsl) makeTempComposeFile(configPath string) (string, error) {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		return "", fmt.Errorf("cannot distinguish caller information")
	}

	basepath := filepath.Dir(filename)
	name := "peer.compose.yml.tpl"
	templatePath := path.Join(basepath, "data", name)
	tmpl, err := template.New(name).ParseFiles(templatePath)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	spec := struct {
		ConfigPath string
		Image      string
		Service    string
	}{
		ConfigPath: configPath,
		Image:      wireguardImage,
		Service:    PeerServiceName,
	}
	if err := tmpl.Execute(buf, spec); err != nil {
		return "", err
	}

	compose := buf.Bytes()
	fileName := fmt.Sprintf("/tmp/peer-%s.compose.yml", randomString())
	if err := os.WriteFile(fileName, compose, 0644); err != nil {
		return "", err
	}

	return fileName, nil
}

// dump peer configuration into temporary file and return its path
func (dsl Dsl) makeTempConfig(peerConfig string) (string, error) {
	path := fmt.Sprintf("/tmp/peer-%s.conf", randomString())
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}

	data := []byte(peerConfig)
	if err := os.WriteFile(file.Name(), data, 0644); err != nil {
		return "", err
	}

	return file.Name(), nil
}

func randomString() string {
	const resultLength = 10
	const charset = "abcdefghijklmnopqrstuvwxyz"

	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomBytes := make([]byte, resultLength)
	for i := range randomBytes {
		randomBytes[i] = charset[rand.Intn(len(charset))]
	}

	return string(randomBytes)
}

func (dsl Dsl) readObjectFromFile(path string) (
	k8sRuntime.Object, *schema.GroupVersionKind, error) {

	sampleBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var rawObj k8sRuntime.RawExtension
	reader := bytes.NewReader(sampleBytes)
	decoder := yamlutil.NewYAMLOrJSONDecoder(reader, 100)
	if err := decoder.Decode(&rawObj); err != nil {
		return nil, nil, err
	}

	scheme := unstructured.UnstructuredJSONScheme
	srlz := yaml.NewDecodingSerializer(scheme)
	obj, gvk, err := srlz.Decode(rawObj.Raw, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	return obj, gvk, nil
}

func (dsl Dsl) objectToUnstructured(
	obj k8sRuntime.Object, gvk *schema.GroupVersionKind) (
	*unstructured.Unstructured, *schema.GroupVersionResource, error) {

	disc := dsl.apiExtensionsClient.Discovery()
	gr, err := restmapper.GetAPIGroupResources(disc)
	if err != nil {
		return nil, nil, err
	}

	mapper := restmapper.NewDiscoveryRESTMapper(gr)
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, nil, err
	}

	conventer := k8sRuntime.DefaultUnstructuredConverter
	objMap, err := conventer.ToUnstructured(obj)
	if err != nil {
		return nil, nil, err
	}

	unstrcd := &unstructured.Unstructured{
		Object: objMap,
	}

	return unstrcd, &mapping.Resource, nil
}

func makeStaticClient() (*kubernetes.Clientset, error) {
	kubeConfig, err := makeKubeConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func makeDynamicClient() (*dynamic.DynamicClient, error) {
	kubeConfig, err := makeKubeConfig()
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func makeKubeConfig() (*rest.Config, error) {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	kubeConfigPath := filepath.Join(userHomeDir, ".kube", "config")
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, err
	}

	return kubeConfig, nil
}
