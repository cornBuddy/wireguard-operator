.PHONY: lint
lint: tidy fmt vet

.PHONY: vendor
vendor: update tidy

.PHONY: env
env: clean
	cd third_party/terraform/flavours/$(FLAVOUR)/ \
		&& terraform init \
		&& terraform plan \
		&& terraform apply -auto-approve

.PHONY: clean
clean:
	find . -type f -name "terraform.tfstate" -exec rm -rf {} +
	find . -type f -name ".terraform.lock.hcl" -exec rm -rf {} +
	find . -type d -name ".terraform" -exec rm -rf {} +

.PHONY: nuke
nuke:
	docker compose \
		--file third_party/aws-nuke/docker-compose.yml up \
		--remove-orphans

.PHONY: smoke
smoke:
	go test -v ./smoke

.PHONY: acceptance
acceptance:
	go test -v ./acceptance

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
