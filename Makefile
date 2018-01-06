.DEFAULT_GOAL := build

GOPATH := $(shell go env | grep GOPATH | sed 's/GOPATH="\(.*\)"/\1/')
PATH := $(GOPATH)/bin:$(PATH)
export $(PATH)

BINARY=byteirc
LD_FLAGS += -s -w

update-deps: fetch
	$(GOPATH)/bin/govendor add -v +external
	$(GOPATH)/bin/govendor remove -v +unused
	$(GOPATH)/bin/govendor update -v +external

upgrade-deps: update-deps
	$(GOPATH)/bin/govendor fetch -v +vendor

fetch:
	test -f $(GOPATH)/bin/govendor || go get -u -v github.com/kardianos/govendor
	test -f $(GOPATH)/bin/rice || go get -u -v github.com/GeertJohan/go.rice/rice
	$(GOPATH)/bin/govendor sync

clean:
	/bin/rm -rfv ${BINARY} rice-box.go

generate:
	$(GOPATH)/bin/rice -v embed-go

debug: clean fetch
	go run *.go -b ":8080" -d

build: fetch clean generate
	go build -ldflags "${LD_FLAGS}" -i -v -o ${BINARY}
