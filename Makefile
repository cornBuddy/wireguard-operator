.PHONY: pre-commit
pre-commit:
	pre-commit install
	pre-commit run --verbose --all-files --show-diff-on-failure

.PHONY: run
run:
	@$(MAKE) -C src run

.PHONY: lint
lint:
	@$(MAKE) -C src lint manifests generate
	@$(MAKE) -C spec lint

.PHONY: test
test:
	@$(MAKE) -C src test

.PHONY: vendor
vendor:
	@$(MAKE) -C src vendor
	@$(MAKE) -C spec vendor

.PHONY: spec
smoke:
	@$(MAKE) -C spec smoke

.PHONY: deploy
deploy:
	@$(MAKE) -C src deploy

.PHONY: undeploy
undeploy:
	@$(MAKE) -C src undeploy

.PHONY: clean
clean:
	@$(MAKE) -C src clean
	@$(MAKE) -C spec run
