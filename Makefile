# Exporting bin folder to the path for makefile
export PATH   := $(PWD)/bin:$(PATH)
# Default Shell
export SHELL  := bash
# Type of OS: Linux or Darwin.
export OSTYPE := $(shell uname -s)


# --- Tooling & Variables ----------------------------------------------------------------
include ./misc/make/tools.Makefile

# Install local dependencies
install-deps: mockery

deps: $(MOCKERY) ## Checks for Global Development Dependencies.
deps:
	@echo "Required Tools Are Available"

generate-mocks: mockery
	bin/mockery --config mockery.yaml

run:
	go run app/main.go

redis-start:
	docker run -d --name redis-stack -p 6379:6379 -p 8001:8001 -v ./redis-cache/:/data redis/redis-stack:7.2.0-v3

redis-stop:
	docker container rm -f redis-stack