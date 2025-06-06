###############################################################################
###                                  Test                                   ###
###############################################################################

test-help:
	@echo "test subcommands"
	@echo ""
	@echo "Usage:"
	@echo "  make test-[command]"
	@echo ""
	@echo "Available Commands:"
	@echo "  unit           Run unit tests"


test: test-help

test-unit:
	@VERSION=$(VERSION) go test -mod=readonly $(PACKAGES_UNIT)

.PHONY: test-unit
