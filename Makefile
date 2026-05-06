
BASE_IMG ?= system-tests
BASE_TAG ?= latest

GO_PACKAGES=$(shell go list ./... | grep -v vendor)
.PHONY: lint \
        deps-update \
        vet
vet:
	go vet ${GO_PACKAGES}

lint:
	@echo "Running go lint"
	scripts/golangci-lint.sh

deps-update:
	go mod tidy && \
	go mod vendor

# ECO_GOINFRA_BRANCH: optional branch to sync from (e.g. release-4.20). Empty = default branch.
ECO_GOINFRA_BRANCH ?=
sync-eco-goinfra:
ifneq ($(ECO_GOINFRA_BRANCH),)
	go get github.com/rh-ecosystem-edge/eco-goinfra@$(ECO_GOINFRA_BRANCH)
else
	go get github.com/rh-ecosystem-edge/eco-goinfra
endif
	go mod tidy
	go mod vendor

install-ginkgo:
	scripts/install-ginkgo.sh

build-docker-image:
	@echo "Building docker image"
	podman build -t "${BASE_IMG}:${BASE_TAG}" -f Dockerfile

install: deps-update install-ginkgo
	@echo "Installing needed dependencies"

run-tests:
	@echo "Executing test-runner script"
	scripts/test-runner.sh

run-internal-pkg-unit-tests:
	@echo "Executing internal package unit tests"
	UNIT_TEST=true go test -v ./tests/internal/...

# Note: To add more unit tests for more packages, add corresponding targets here
test: run-internal-pkg-unit-tests
	
coverage-html: test
	go tool cover -html cover.out
