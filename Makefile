
REGISTRY ?= replicated

PLUGIN_NAME ?= local-volume-provider
PLUGIN_IMAGE    ?= $(REGISTRY)/$(PLUGIN_NAME)

FS_NAME ?= local-volume-fileserver
FS_IMAGE    ?= $(REGISTRY)/$(FS_NAME)

VERSION  ?= main 
CURRENT_USER := $(shell id -u -n)

GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# local builds the binary using 'go build' in the local environment.
.PHONY: plugin
local: build-dirs
	CGO_ENABLED=0 go build -v -o _output/bin/$(GOOS)/$(GOARCH) ./cmd/local-volume-provider

.PHONY: fileserver
fileserver: build-dirs
	CGO_ENABLED=0 go build -v -o _output/bin/$(GOOS)/$(GOARCH) ./cmd/local-volume-fileserver

# test runs unit tests using 'go test' in the local environment.
.PHONY: test
test:
	CGO_ENABLED=0 go test -v -timeout 60s ./...

# ci is a convenience target for CI builds.
.PHONY: ci
ci: verify-modules local test

.PHONY: container-plugin
container-plugin:
	docker build -t $(PLUGIN_IMAGE):$(VERSION) -f deploy/local-volume-provider/Dockerfile .

.PHONY: container-fileserver
container-fileserver:
	docker build -t $(FS_IMAGE):$(VERSION) -f deploy/local-volume-fileserver/Dockerfile .

.PHONY: containers
containers: container-plugin container-fileserver

# push pushes the Docker image to its registry.
.PHONY: push
push:
	@docker push $(PLUGIN_IMAGE):$(VERSION)
	@docker push $(FS_IMAGE):$(VERSION)
ifeq ($(TAG_LATEST), true)
	docker tag $(PLUGIN_IMAGE):$(VERSION) $(PLUGIN_IMAGE):latest
	docker push $(PLUGIN_IMAGE):latest

	docker tag $(FS_IMAGE):$(VERSION) $(FS_IMAGE):latest
	docker push $(FS_IMAGE):latest
endif

.PHONY ttl.sh-fileserver:
ttl.sh-plugin:
	docker build -t $(CURRENT_USER)/$(PLUGIN_NAME):2h -f deploy/local-volume-provider/Dockerfile .
	docker tag $(CURRENT_USER)/$(PLUGIN_NAME):2h ttl.sh/$(CURRENT_USER)/$(PLUGIN_NAME):2h
	@docker push ttl.sh/$(CURRENT_USER)/$(PLUGIN_NAME):2h

ttl.sh-fileserver:
	docker build -t $(CURRENT_USER)/$(FS_NAME):2h -f deploy/local-volume-fileserver/Dockerfile  .
	docker tag $(CURRENT_USER)/$(FS_NAME):2h ttl.sh/$(CURRENT_USER)/$(FS_NAME):2h
	@docker push ttl.sh/$(CURRENT_USER)/$(FS_NAME):2h

.PHONY: ttl.sh
ttl.sh: ttl.sh-plugin ttl.sh-fileserver

# modules updates Go module files
.PHONY: modules
modules:
	go mod tidy

# verify-modules ensures Go module files are up to date
.PHONY: verify-modules
verify-modules: modules
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please commit the changes to go.mod and go.sum"; exit 1; \
	fi

# build-dirs creates the necessary directories for a build in the local environment.
.PHONY: build-dirs
build-dirs:
	@mkdir -p _output/bin/$(GOOS)/$(GOARCH)

# clean removes build artifacts from the local environment.
.PHONY: clean
clean:
	@echo "cleaning"
	rm -rf _output
