.PHONY: lint
lint: tidy fmt vet

.PHONY: vendor
vendor: update tidy

.PHONY: env
env:
	cd vendor/terraform/flavours/$(FLAVOUR)/ \
		&& terraform init \
		&& terraform plan \
		&& terraform apply -auto-approve

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
tidy:
	go mod tidy

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: update
update:
	go get -u ./...
