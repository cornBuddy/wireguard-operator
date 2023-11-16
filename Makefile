# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command
# fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

LOCALBIN ?= $(shell pwd)/bin
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
KUSTOMIZE ?= $(LOCALBIN)/kustomize

IMG_BASE ?= wireguard-operator
TAG ?= latest
IMG ?= ${IMG_BASE}:${TAG}

.PHONY: all
all: samples run

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: docker
docker: ## Build docker image
	docker build --tag ${IMG} .

.PHONY: clean
clean: uninstall ## Cleans up development environment
	@$(MAKE) -C spec clean
	docker rmi $(IMG)
	rm $(BIN_PATH) || true
	minikube delete

.PHONY: minikube
minikube: ## Sets up test k8s cluster
	minikube start \
		--cpus='2' \
		--memory='2g' \
		--addons=ingress \
		--extra-config="kubelet.allowed-unsafe-sysctls=net.ipv4.*,net.ipv6.*" \
		--driver=docker


.PHONY: tunnel
tunnel: ## Enable tunnelling for local testing
	pkill minikube || true
	minikube tunnel &

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	@$(MAKE) -C spec vet
	go vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy against code.
	@$(MAKE) -C spec tidy
	go mod tidy

.PHONY: update
update: ## Updates all packates
	go get -u ./...

.PHONY: vendor
vendor: update tidy ## Packages update and `go mod tidy`

##@ Build

.PHONY: build
BIN_PATH ?= "./wireguard-operator"
build: generate fmt vet ## Build manager binary.
	go build -o $(BIN_PATH) main.go

.PHONY: run
run: tunnel manifests generate install fmt vet test ## Run a controller from your host.
	go run ./main.go

.PHONY: generate
generate: controller-gen ## Generates some golang code for CRDs
	$(CONTROLLER_GEN) object:headerFile="" paths="./..."

.PHONY: manifests
manifests: controller-gen ## Generate CRDs
	$(CONTROLLER_GEN) rbac:roleName=wireguard-operator crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

## Location to install dependencies to
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_TOOLS_VERSION ?= v0.10.0

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

##@ Testing
#
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.1
.PHONY: test
test: manifests generate fmt vet tidy envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test -v $(shell go list ./... | grep -v /spec) -coverprofile coverage.out

.PHONY: smoke
smoke: ## Perform smoke tests
	# @$(MAKE) -C spec smoke
	echo "dummy target"

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs
	$(KUSTOMIZE) build config/crd | kubectl apply -f -
	# NOTE: this is workaroud for acceptance tests
	# wait some time to ensure that crds are delivered to the cluster
	sleep 10

.PHONY: samples
samples: install ## Deploy samples
	$(KUSTOMIZE) build config/samples | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs
	$(KUSTOMIZE) build config/samples | kubectl delete --ignore-not-found=false -f -
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=false -f -

.PHONY: deploy
deploy: manifests kustomize install ## Deploy controller
	cd config/manager && $(KUSTOMIZE) edit set image wireguard-operator=${IMG}
	kubectl create namespace wireguard-operator --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply --server-side -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.66.0/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=false -f -
	kubectl delete namespace wireguard-operator
