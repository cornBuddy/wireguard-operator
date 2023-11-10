package spec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	namespace      = "default"
	wgSampleFile   = "../config/samples/wireguard.yaml"
	peerSampleFile = "../config/samples/wireguardpeer.yaml"
)

var (
	ctx = context.TODO()
)

type Dsl struct {
	ApiExtensionsClient *clientset.Clientset
	DynamicClient       *dynamic.DynamicClient
	StaticClient        *kubernetes.Clientset
}

func (dsl Dsl) MakeSamples() error {
	// https: //gist.github.com/pytimer/0ad436972a073bb37b8b6b8b474520fc
	for _, sample := range []string{wgSampleFile, peerSampleFile} {
		obj, gvk, err := dsl.readObjectFromFile(sample)
		if err != nil {
			return err
		}

		unstrcd, res, err := dsl.objectToUnstructured(obj, gvk)
		if err != nil {
			return err
		}

		opts := metav1.CreateOptions{}
		dri := dsl.DynamicClient.Resource(*res).Namespace(namespace)
		if _, err := dri.Create(ctx, unstrcd, opts); err != nil {
			return err
		}
	}

	return nil
}

func (dsl Dsl) DeleteSamples() error {
	for _, sample := range []string{wgSampleFile, peerSampleFile} {
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
		dri := dsl.DynamicClient.Resource(*res).Namespace(namespace)
		if err := dri.Delete(ctx, name, opts); err != nil {
			return err
		}
	}

	return nil
}

func (dsl Dsl) readObjectFromFile(path string) (
	runtime.Object, *schema.GroupVersionKind, error) {

	sampleBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var rawObj runtime.RawExtension
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
	obj runtime.Object, gvk *schema.GroupVersionKind) (
	*unstructured.Unstructured, *schema.GroupVersionResource, error) {

	disc := dsl.ApiExtensionsClient.Discovery()
	gr, err := restmapper.GetAPIGroupResources(disc)
	if err != nil {
		return nil, nil, err
	}

	mapper := restmapper.NewDiscoveryRESTMapper(gr)
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, nil, err
	}

	conventer := runtime.DefaultUnstructuredConverter
	objMap, err := conventer.ToUnstructured(obj)
	if err != nil {
		return nil, nil, err
	}

	unstrcd := &unstructured.Unstructured{
		Object: objMap,
	}

	return unstrcd, &mapping.Resource, nil
}

func MakeStaticClient() (*kubernetes.Clientset, error) {
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

func MakeDynamicClient() (*dynamic.DynamicClient, error) {
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

func MakeApiExtensionsClient() (*clientset.Clientset, error) {
	kubeConfig, err := makeKubeConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := clientset.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return clientset, nil
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
