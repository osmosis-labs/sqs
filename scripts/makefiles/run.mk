###############################################################################
###                                  Run                                    ###
###############################################################################

GO_RUN_FLAGS ?=
GO_RUN_CMD = go run $(GO_RUN_FLAGS) -ldflags="-X github.com/osmosis-labs/sqs/version=${VERSION}" app/*.go  --config config.json

run-help:
	@echo "run subcommands"
	@echo ""
	@echo "Usage:"
	@echo "  make run-[command]"
	@echo ""
	@echo "Available Commands:"
	@echo "  default    Run service normally"
	@echo "  state      Run from the state files"
	@echo "  docker     Run in a Docker container"
	@echo "  race       Run with race enabled"

run: run-default

run-default:
	$(GO_RUN_CMD)

run-state:
	$(GO_RUN_CMD) --serve-from-state

run-docker:
	$(DOCKER) rm -f sqs
	$(DOCKER) run -d --name sqs -p 9092:9092 -p 26657:26657 -v $(PWD)/config.json:/osmosis/config.json:ro --net host osmolabs/sqs:local --config /osmosis/config.json
	$(DOCKER) logs -f sqs

run-race:
	$(MAKE) GO_RUN_FLAGS="-race" run-default
