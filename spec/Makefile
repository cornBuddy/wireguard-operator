.PHONY: lint
lint: tidy fmt vet ## Static analisys of the code

.PHONY: vendor
vendor: update tidy ## Packages update and `go mod tidy`

.PHONY: clean
clean:
	docker compose --file vendor/aws-nuke/docker-compose.yml up --remove-orphans

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

.PHONY: update
update: ## Updates all packates
	go get -u ./...
