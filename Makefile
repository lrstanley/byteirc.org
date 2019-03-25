.DEFAULT_GOAL := build

GOPATH := $(shell go env | grep GOPATH | sed 's/GOPATH="\(.*\)"/\1/')
PATH := $(GOPATH)/bin:$(PATH)
export $(PATH)

# enable Go 1.11.x module support.
export GO111MODULE=on

BINARY=byteirc

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}'

fetch: ## Fetches the necessary dependencies to build.
	test -f $(GOPATH)/bin/goreleaser || go get -u -v github.com/goreleaser/goreleaser
	test -f $(GOPATH)/bin/rice || go get -u -v github.com/GeertJohan/go.rice/rice
	go mod download
	go mod tidy
	go mod vendor

clean: ## Cleans up generated files/folders from the build.
	/bin/rm -rfv "dist/" "${BINARY}" "rice-box.go"

generate:
	$(GOPATH)/bin/rice -v embed-go

debug: clean fetch
	go run *.go -b ":8080" -d

build: fetch clean generate
	go build -ldflags '-d -s -w' -tags netgo -installsuffix netgo -v -o "${BINARY}"
