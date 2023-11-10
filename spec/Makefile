.PHONY: smoke
smoke:
	go test -v ./main_test.go ./smoke_test.go

.PHONY: vet
vet:
	go vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy against code.
	go mod tidy

.PHONY: update
update: ## Updates all packates
	go get -u ./...

.PHONY: vendor
vendor: update tidy ## Packages update and `go mod tidy`
