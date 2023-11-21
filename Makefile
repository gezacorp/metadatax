REPO_ROOT=$(shell git rev-parse --show-toplevel)

include common.mk

.PHONY: help
.DEFAULT_GOAL := help
help:
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: tidy-all
tidy-all:	## go mod tidy all go modules
	./scripts/gomodules.sh --with-file Makefile -- make tidy

.PHONY: fmt-all
fmt-all:	## go fmt all go modules
	./scripts/gomodules.sh --with-file Makefile -- make fmt

.PHONY: vet-all
vet-all:	## go vet all go modules
	./scripts/gomodules.sh --with-file Makefile -- make vet

.PHONY: test-all
test-all:	## go fmt all go modules
	./scripts/gomodules.sh --with-file Makefile -- make test

.PHONY: lint-all
lint-all: ${REPO_ROOT}/bin/golangci-lint ## lint the whole repo
	./scripts/gomodules.sh --parallel 1 --with-file Makefile -- make lint

.PHONY: lint-fix-all
lint-fix-all: ${REPO_ROOT}/bin/golangci-lint ## lint --fix the whole repo
	./scripts/gomodules.sh --parallel 1 --with-file Makefile -- make lint-fix
