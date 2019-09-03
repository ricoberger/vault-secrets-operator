BRANCH       ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILDTIME    ?= $(shell date '+%Y-%m-%d@%H:%M:%S')
BUILDUSER    ?= $(shell id -un)
DOCKER_IMAGE ?= vault-secrets-operator
REPO         ?= github.com/ricoberger/vault-secrets-operator
REVISION     ?= $(shell git rev-parse HEAD)
VERSION      ?= $(shell git describe --tags)

.PHONY: build release release-major release-minor release-patch

build:
	operator-sdk build --go-build-args "-ldflags -X=${REPO}/version.BuildInformation=${VERSION},${REVISION},${BRANCH},${BUILDUSER},${BUILDTIME}" $(DOCKER_IMAGE):${VERSION}

release: build
	docker tag $(DOCKER_IMAGE):${VERSION} ricoberger/$(DOCKER_IMAGE):${VERSION}
	docker tag $(DOCKER_IMAGE):${VERSION} docker.pkg.github.com/ricoberger/vault-secrets-operator/$(DOCKER_IMAGE):${VERSION}
	docker push ricoberger/$(DOCKER_IMAGE):${VERSION}
	docker push docker.pkg.github.com/ricoberger/vault-secrets-operator/$(DOCKER_IMAGE):${VERSION}

release-major:
	$(eval OLD_VERSION=$(shell git describe --tags --abbrev=0))
	$(eval MAJORVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1+1".0.0"}'))
	git checkout master
	git pull
	sed -i'.backup' 's/${OLD_VERSION}/${MAJORVERSION}/g' charts/README.md
	sed -i'.backup' 's/${OLD_VERSION}/${MAJORVERSION}/g' charts/vault-secrets-operator/Chart.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${MAJORVERSION}/g' charts/vault-secrets-operator/values.yaml
	rm charts/README.md.backup
	rm charts/vault-secrets-operator/Chart.yaml.backup
	rm charts/vault-secrets-operator/values.yaml.backup
	git add .
	git commit -m 'Prepare release $(MAJORVERSION)'
	git push
	git tag -a $(MAJORVERSION) -m 'release $(MAJORVERSION)'
	git push origin --tags

release-minor:
	$(eval OLD_VERSION=$(shell git describe --tags --abbrev=0))
	$(eval MINORVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1"."$$2+1".0"}'))
	git checkout master
	git pull
	sed -i'.backup' 's/${OLD_VERSION}/${MINORVERSION}/g' charts/README.md
	sed -i'.backup' 's/${OLD_VERSION}/${MINORVERSION}/g' charts/vault-secrets-operator/Chart.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${MINORVERSION}/g' charts/vault-secrets-operator/values.yaml
	rm charts/README.md.backup
	rm charts/vault-secrets-operator/Chart.yaml.backup
	rm charts/vault-secrets-operator/values.yaml.backup
	git add .
	git commit -m 'Prepare release $(MINORVERSION)'
	git push
	git tag -a $(MINORVERSION) -m 'release $(MINORVERSION)'
	git push origin --tags

release-patch:
	$(eval OLD_VERSION=$(shell git describe --tags --abbrev=0))
	$(eval PATCHVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1"."$$2"."$$3+1}'))
	git checkout master
	git pull
	sed -i'.backup' 's/${OLD_VERSION}/${PATCHVERSION}/g' charts/README.md
	sed -i'.backup' 's/${OLD_VERSION}/${PATCHVERSION}/g' charts/vault-secrets-operator/Chart.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${PATCHVERSION}/g' charts/vault-secrets-operator/values.yaml
	rm charts/README.md.backup
	rm charts/vault-secrets-operator/Chart.yaml.backup
	rm charts/vault-secrets-operator/values.yaml.backup
	git add .
	git commit -m 'Prepare release $(PATCHVERSION)'
	git push
	git tag -a $(PATCHVERSION) -m 'release $(PATCHVERSION)'
	git push origin --tags
