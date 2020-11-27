# Current Operator version
VERSION ?= $(shell git describe --tags --abbrev=0)
# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= ricoberger/vault-secrets-operator:$(VERSION)
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:crdVersions={v1},trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Run tests
ENVTEST_ASSETS_DIR = $(shell pwd)/testbin
test: generate fmt vet manifests
	mkdir -p $(ENVTEST_ASSETS_DIR)
	test -f $(ENVTEST_ASSETS_DIR)/setup-envtest.sh || curl -sSLo $(ENVTEST_ASSETS_DIR)/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.6.3/hack/setup-envtest.sh
	bash -c 'source $(ENVTEST_ASSETS_DIR)/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out'

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests kustomize
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: release-major
release-major:
	$(eval OLD_VERSION=$(shell git describe --tags --abbrev=0))
	$(eval MAJORVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1+1".0.0"}'))
	git checkout master
	git pull
	sed -i'.backup' 's/${OLD_VERSION}/${MAJORVERSION}/g' charts/README.md
	sed -i'.backup' 's/${OLD_VERSION}/${MAJORVERSION}/g' charts/vault-secrets-operator/Chart.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${MAJORVERSION}/g' charts/vault-secrets-operator/values.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${MAJORVERSION}/g' config/manager/deploy.yaml
	rm charts/README.md.backup
	rm charts/vault-secrets-operator/Chart.yaml.backup
	rm charts/vault-secrets-operator/values.yaml.backup
	rm config/manager/deploy.yaml.backup
	git add .
	git commit -m 'Prepare release $(MAJORVERSION)'
	git push
	git tag -a $(MAJORVERSION) -m 'release $(MAJORVERSION)'
	git push origin --tags

.PHONY: release-minor
release-minor:
	$(eval OLD_VERSION=$(shell git describe --tags --abbrev=0))
	$(eval MINORVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1"."$$2+1".0"}'))
	git checkout master
	git pull
	sed -i'.backup' 's/${OLD_VERSION}/${MINORVERSION}/g' charts/README.md
	sed -i'.backup' 's/${OLD_VERSION}/${MINORVERSION}/g' charts/vault-secrets-operator/Chart.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${MINORVERSION}/g' charts/vault-secrets-operator/values.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${MINORVERSION}/g' config/manager/deploy.yaml
	rm charts/README.md.backup
	rm charts/vault-secrets-operator/Chart.yaml.backup
	rm charts/vault-secrets-operator/values.yaml.backup
	rm config/manager/deploy.yaml.backup
	git add .
	git commit -m 'Prepare release $(MINORVERSION)'
	git push
	git tag -a $(MINORVERSION) -m 'release $(MINORVERSION)'
	git push origin --tags

.PHONY: release-patch
release-patch:
	$(eval OLD_VERSION=$(shell git describe --tags --abbrev=0))
	$(eval PATCHVERSION=$(shell git describe --tags --abbrev=0 | sed s/v// | awk -F. '{print $$1"."$$2"."$$3+1}'))
	git checkout master
	git pull
	sed -i'.backup' 's/${OLD_VERSION}/${PATCHVERSION}/g' charts/README.md
	sed -i'.backup' 's/${OLD_VERSION}/${PATCHVERSION}/g' charts/vault-secrets-operator/Chart.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${PATCHVERSION}/g' charts/vault-secrets-operator/values.yaml
	sed -i'.backup' 's/${OLD_VERSION}/${PATCHVERSION}/g' config/manager/deploy.yaml
	rm charts/README.md.backup
	rm charts/vault-secrets-operator/Chart.yaml.backup
	rm charts/vault-secrets-operator/values.yaml.backup
	rm config/manager/deploy.yaml.backup
	git add .
	git commit -m 'Prepare release $(PATCHVERSION)'
	git push
	git tag -a $(PATCHVERSION) -m 'release $(PATCHVERSION)'
	git push origin --tags
