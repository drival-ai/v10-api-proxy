VERSION ?= $(shell git describe --tags --always --dirty --match=v* 2> /dev/null || cat $(CURDIR)/.version 2> /dev/null || echo v0)
BLDVER = module:$(MODULE),version:$(VERSION),build:$(CIRCLE_BUILD_NUM)
BASE = $(CURDIR)
MODULE = v10-api-proxy

.PHONY: all $(MODULE) debug
all: version $(MODULE)

$(MODULE):| $(BASE)
	@GO111MODULE=on GOFLAGS=-mod=vendor go build -v -o $(BASE)/bin/$@

$(BASE):
	@mkdir -p $(dir $@)

debug: $(BASE)
	@go build -gcflags="-N -l" -v -o $(BASE)/bin/$(MODULE)

# misc
prune:
	@docker system prune -f

.PHONY: clean version list
clean:
	@rm -rfv bin;
	$(MAKE) dockerclean

dockerclean:
	@docker rmi $(docker images --filter "dangling=true" -q --no-trunc)

version:
	@echo "Version: $(VERSION)"

list:
	@$(MAKE) -pRrq -f $(lastword $(MAKEFILE_LIST)) : 2>/dev/null | awk -v RS= -F: '/^# File/,/^# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' | sort | egrep -v -e '^[^[:alnum:]]' -e '^$@$$' | xargs
