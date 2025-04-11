package dsl

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"text/template"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	samples        = "../../../src/config/samples/"
	wireguardImage = "linuxserver/wireguard:1.0.20210914"
)

// performs given kubectl operation for rendered samples
func (dsl Dsl) kustomizeSamples(kubectlArg string) error {
	_, b, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("cannot distinguish caller information")
	}

	filename := filepath.Dir(b)
	samples := path.Join(filename, samples)
	bash := fmt.Sprintf(
		"kustomize build %s | kubectl %s -f -",
		samples, kubectlArg,
	)
	cmd := exec.Command("bash", "-c", bash)
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

// generate docker compose file for peer with configuration mounted into
// container
func (dsl Dsl) makeTempComposeFile(configPath, network string) (string, error) {
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
		ConfigPath  string
		Image       string
		Service     string
		NetworkName string
	}{
		ConfigPath:  configPath,
		Image:       wireguardImage,
		Service:     PeerServiceName,
		NetworkName: network,
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
