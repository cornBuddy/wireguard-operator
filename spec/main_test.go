package spec

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/strslice"
	dockerClient "github.com/docker/docker/client"

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
	T                   *testing.T
	DockerClient        *dockerClient.Client
	ApiExtensionsClient *clientset.Clientset
	DynamicClient       *dynamic.DynamicClient
	StaticClient        *kubernetes.Clientset
}

// starts a process withing container with the given id, returns cmd output
func (dsl Dsl) Exec(id string, cmd []string) (string, error) {
	cli := dsl.DockerClient
	execCfg := types.ExecConfig{
		Cmd:          cmd,
		Tty:          true,
		AttachStdout: true,
		Detach:       true,
		AttachStdin:  true,
	}
	exec, err := cli.ContainerExecCreate(ctx, id, execCfg)
	if err != nil {
		return "", err
	}
	dsl.T.Log("exec created")

	execCheck := types.ExecStartCheck{}
	attach, err := cli.ContainerExecAttach(ctx, exec.ID, execCheck)
	if err != nil {
		return "", err
	}
	defer attach.Close()
	dsl.T.Log("exec attached")

	buf := new(strings.Builder)
	if _, err := io.Copy(buf, attach.Reader); err != nil {
		return "", err
	}
	dsl.T.Log("exec output copied")

	return buf.String(), nil
}

// spawns peer as a docker container
func (dsl Dsl) SpawnPeer(peerConfig string) (container.CreateResponse, error) {
	empty := container.CreateResponse{}
	config, err := dsl.makeTempConfig(peerConfig)
	if err != nil {
		return empty, err
	}
	dsl.T.Log("config written")

	const image = "docker.io/linuxserver/wireguard:latest"
	cli := dsl.DockerClient
	pullOpts := types.ImagePullOptions{}
	reader, err := cli.ImagePull(ctx, image, pullOpts)
	if err != nil {
		return empty, err
	}
	dsl.T.Log("pulling image:")
	io.Copy(os.Stdout, reader)

	hostConfig := &container.HostConfig{
		CapAdd: strslice.StrSlice{"NET_ADMIN"},
		Mounts: []mount.Mount{{
			Type:   mount.TypeBind,
			Source: config.Name(),
			Target: "/config/wg_confs/wg0.conf",
		}},
		Sysctls: map[string]string{
			"net.ipv4.conf.all.src_valid_mark": "1",
		},
	}
	container, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
	}, hostConfig, nil, nil, "peer-test")
	if err != nil {
		return empty, err
	}
	dsl.T.Log("container created")

	startOpts := types.ContainerStartOptions{}
	if err := cli.ContainerStart(ctx, container.ID, startOpts); err != nil {
		return empty, err
	}
	dsl.T.Log("container started")

	return container, nil
}

func (dsl Dsl) makeTempConfig(peerConfig string) (*os.File, error) {
	path := "/tmp/test-peer.conf"
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	data := []byte(peerConfig)
	if err := os.WriteFile(file.Name(), data, 0644); err != nil {
		return nil, err
	}

	return file, nil
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

func NewDsl(t *testing.T) (*Dsl, error) {
	apiExtClient, err := makeApiExtensionsClient()
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

	dockerClient, err := dockerClient.NewClientWithOpts()
	if err != nil {
		return nil, err
	}

	return &Dsl{
		T:                   t,
		DockerClient:        dockerClient,
		ApiExtensionsClient: apiExtClient,
		DynamicClient:       dynamicClient,
		StaticClient:        staticClient,
	}, nil
}

func makeStaticClient() (*kubernetes.Clientset, error) {
	kubeConfig, err := makeKubeConfig()
	if err != nil {
		return nil, err
	}

	dockerClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return dockerClient, nil
}

func makeDynamicClient() (*dynamic.DynamicClient, error) {
	kubeConfig, err := makeKubeConfig()
	if err != nil {
		return nil, err
	}

	dockerClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return dockerClient, nil
}

func makeApiExtensionsClient() (*clientset.Clientset, error) {
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
