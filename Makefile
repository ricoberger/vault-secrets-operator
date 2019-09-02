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
	docker tag $(DOCKER_IMAGE):${VERSION} docker.pkg.github.com/ricoberger/packageregistry/$(DOCKER_IMAGE):${VERSION}
	docker push ricoberger/$(DOCKER_IMAGE):${VERSION}
	docker push docker.pkg.github.com/ricoberger/packageregistry/$(DOCKER_IMAGE):${VERSION}

release-major:
	$(eval MAJORVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1+1".0.0"}'))
	git checkout master
	git pull
	git tag -a $(MAJORVERSION) -m 'release $(MAJORVERSION)'
	git push origin --tags

release-minor:
	$(eval MINORVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1"."$$2+1".0"}'))
	git checkout master
	git pull
	git tag -a $(MINORVERSION) -m 'release $(MINORVERSION)'
	git push origin --tags

release-patch:
	$(eval PATCHVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1"."$$2"."$$3+1}'))
	git checkout master
	git pull
	git tag -a $(PATCHVERSION) -m 'release $(PATCHVERSION)'
	git push origin --tags
