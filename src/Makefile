# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command
# fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec


DEPLOY ?= ./config

CONTROLLER_GEN ?= controller-gen
KUSTOMIZE ?= kustomize

BIN_PATH ?= "./wireguard-operator"
IMG_BASE ?= wireguard-operator
TAG ?= latest
IMG ?= ${IMG_BASE}:${TAG}

##@ General

.PHONY: default
default: clean run ## Fresh start of the operator from local sources.

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: run
run: manifests generate install fmt vet ## Run a controller from your host.
	go run ./main.go

.PHONY: pre-commit
pre-commit:
	pre-commit install
	pre-commit run --verbose --all-files --show-diff-on-failure

.PHONY: clean
clean: uninstall ## Cleans up development environment
	- docker rmi $(IMG)
	- rm $(BIN_PATH)

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy against code.
	go mod tidy

.PHONY: update
update: ## Updates all packates
	go get -u ./...

.PHONY: vendor
vendor: update tidy ## Packages update and `go mod tidy`

.PHONY: lint
lint: tidy fmt vet ## Static analisys of the code

LOCALBIN ?= "bin"
.PHONY: envtest
envtest: ## Installs envtest-setup locally if necessary.
	mkdir -p $(LOCALBIN)
	command -v setup-envtest \
		|| go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

ENVTEST_K8S_VERSION ?= 1.30.0
KUBEBUILDER_ASSETS = $(shell setup-envtest use --force --bin-dir $(LOCALBIN) --print path $(ENVTEST_K8S_VERSION))
.PHONY: test ## Run unit tests
test: manifests generate envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="../$(KUBEBUILDER_ASSETS)" \
		TEST_ASSET_ETCD="../$(KUBEBUILDER_ASSETS)/etcd" \
		TEST_ASSET_KUBECTL="../$(KUBEBUILDER_ASSETS)/kubectl" \
		TEST_ASSET_KUBE_APISERVER="../$(KUBEBUILDER_ASSETS)/kube-apiserver" \
		go test -coverprofile=coverage.out -cover ./...

##@ Build

.PHONY: docker
docker: ## Build docker image
	docker build --tag ${IMG} .

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o $(BIN_PATH) main.go

.PHONY: generate
generate: controller-gen ## Generates some golang code for CRDs
	$(CONTROLLER_GEN) object:headerFile="" paths="./..."

.PHONY: manifests
manifests: controller-gen ## Generate CRDs
	$(CONTROLLER_GEN) \
		rbac:roleName=wireguard-operator \
		crd paths="./..." \
		output:crd:artifacts:config=$(DEPLOY)/crd/bases

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	command -v kustomize \
		|| curl -s $(KUSTOMIZE_INSTALL_SCRIPT)

.PHONY: controller-gen
controller-gen:
	command -v controller-gen \
		|| go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs
	$(KUSTOMIZE) build $(DEPLOY)/crd | kubectl apply -f -

.PHONY: samples
samples: install ## Deploy samples
	$(KUSTOMIZE) build $(DEPLOY)/samples | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs
	- $(KUSTOMIZE) build $(DEPLOY)/samples | kubectl delete --ignore-not-found=true -f -
	- $(KUSTOMIZE) build $(DEPLOY)/crd | kubectl delete --ignore-not-found=true -f -

.PHONY: deploy
deploy: manifests kustomize install ## Deploy controller
	cd $(DEPLOY)/manager && $(KUSTOMIZE) edit set image wireguard-operator=${IMG}
	kubectl create namespace wireguard-operator --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply --server-side -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.66.0/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml
	$(KUSTOMIZE) build $(DEPLOY)/default | kubectl apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller
	- $(KUSTOMIZE) build $(DEPLOY)/default | kubectl delete --ignore-not-found=false -f -
	- kubectl delete namespace wireguard-operator
