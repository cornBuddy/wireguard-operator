# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command
# fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

CONTROLLER_GEN ?= controller-gen
KUSTOMIZE ?= kustomize

BIN_PATH ?= "./wireguard-operator"
DEPLOY ?= ./config

IMAGE ?= wireguard-operator
TAG ?= latest
IMG ?= ${IMAGE}:${TAG}

##@ Development

.PHONY: default
default: clean run ## Fresh start of the operator from local sources

.PHONY: run
run: fmt vet install ## Run a controller from your host
	POD_NAMESPACE=default go run ./main.go

.PHONY: clean
clean: uninstall undeploy ## Cleans up development environment
	- minikube delete
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

.PHONY: minikube
minikube: ## Starts minikube for integration testing
	kubectl get no || minikube start
	- pkill minikube
	minikube tunnel &> /dev/null &
	sleep 5

.PHONY: test
test: minikube install ## Runs unit and integration tests
	go test -cover ./...

##@ Build

.PHONY: docker
docker: ## Build docker image
	docker buildx bake --push

.PHONY: build
build: generate fmt vet ## Build operator
	go build -o $(BIN_PATH) main.go

.PHONY: generate
generate: controller-gen kustomize ## Generates some golang code for CRDs
	$(CONTROLLER_GEN) object:headerFile="" paths="./..."
	$(KUSTOMIZE) build $(DEPLOY)/default > $(DEPLOY)/manifest.yml

.PHONY: manifests
manifests: controller-gen ## Generate CRDs
	$(CONTROLLER_GEN) \
		rbac:roleName=wireguard-operator \
		crd paths="./..." \
		output:crd:artifacts:config=$(DEPLOY)/crd/bases

KUSTOMIZE_INSTALL ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	command -v kustomize || curl -s $(KUSTOMIZE_INSTALL)

.PHONY: controller-gen
controller-gen:
	command -v controller-gen \
		|| go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

##@ Deployment

.PHONY: install
install: generate manifests kustomize ## Install CRDs
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
	$(KUSTOMIZE) build $(DEPLOY)/default | kubectl apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller
	- $(KUSTOMIZE) build $(DEPLOY)/default | kubectl delete --ignore-not-found=false -f -
