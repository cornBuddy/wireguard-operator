.PHONY: clean
clean:
	- kubectl delete wireguard --all
	- kubectl delete wireguardpeer --all

.PHONY: smoke
smoke: clean
	go test -v ./smoke_test.go

.PHONY: vet
vet:
	go vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy against code.
	go mod tidy

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: lint
lint: tidy fmt vet ## Static analisys of the code

.PHONY: update
update: ## Updates all packates
	go get -u ./...

.PHONY: vendor
vendor: update tidy ## Packages update and `go mod tidy`
