# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command
# fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

KUSTOMIZE_VERSION ?= 5.4.2
ENVTEST_K8S_VERSION ?= 1.29.5

LOCALBIN ?= $(shell pwd)/bin
ENVTEST ?= $(LOCALBIN)/setup-envtest
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
KUSTOMIZE ?= $(LOCALBIN)/kustomize

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
run: manifests generate install fmt vet test ## Run a controller from your host.
	go run ./main.go

.PHONY: pre-commit
pre-commit:
	pre-commit install
	pre-commit run --verbose --all-files --show-diff-on-failure

.PHONY: clean
clean: uninstall ## Cleans up development environment
	@$(MAKE) -C spec clean
	- docker rmi $(IMG)
	- rm $(BIN_PATH)

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
	@$(MAKE) -C spec update
	go get -u ./...

.PHONY: vendor
vendor: update tidy ## Packages update and `go mod tidy`

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
	$(CONTROLLER_GEN) rbac:roleName=wireguard-operator crd paths="./..." output:crd:artifacts:config=config/crd/bases

## Location to install dependencies to
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(KUSTOMIZE_VERSION) $(LOCALBIN); }

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

##@ Testing

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
.PHONY: test
test: manifests generate fmt vet tidy envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test $(shell go list ./... | grep -v /spec) -coverprofile coverage.out

.PHONY: smoke
smoke: ## Perform smoke tests
	@$(MAKE) -C spec smoke

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: samples
samples: install ## Deploy samples
	$(KUSTOMIZE) build config/samples | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs
	- $(KUSTOMIZE) build config/samples | kubectl delete --ignore-not-found=true -f -
	- $(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=true -f -

.PHONY: deploy
deploy: manifests kustomize install ## Deploy controller
	cd config/manager && $(KUSTOMIZE) edit set image wireguard-operator=${IMG}
	kubectl create namespace wireguard-operator --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply --server-side -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.66.0/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller
	- $(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=false -f -
	- kubectl delete namespace wireguard-operator
