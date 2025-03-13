.PHONY: lint
lint:
	@$(MAKE) -C spec lint
	@$(MAKE) -C src lint manifests generate

.PHONY: clean
clean:
	@$(MAKE) -C src clean
	@$(MAKE) -C spec clean nuke

.PHONY: vendor
vendor:
	@$(MAKE) -C src vendor
	@$(MAKE) -C spec vendor
.PHONY: run
run:
	@$(MAKE) -C src run

.PHONY: test
test:
	@$(MAKE) -C src test

.PHONY: docker
docker:
	@$(MAKE) -C src docker

.PHONY: deploy
deploy:
	@$(MAKE) -C src deploy

.PHONY: env
env:
	@$(MAKE) -C spec env

.PHONY: spec
smoke:
	@$(MAKE) -C spec smoke

.PHONY: pre-commit
pre-commit:
	pre-commit install
	pre-commit install --hook-type commit-msg
	pre-commit run --verbose --all-files --show-diff-on-failure
