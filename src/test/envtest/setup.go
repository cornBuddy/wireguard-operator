package envtest

import (
	"path/filepath"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type Cleanup func() error

func SetupEnvtest() (*rest.Config, Cleanup, error) {
	crdPath := filepath.Join(
		"..", "config", "crd", "bases",
	)
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() error {
		return testEnv.Stop()
	}
	return cfg, cleanup, nil
}
