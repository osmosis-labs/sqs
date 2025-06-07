###############################################################################
###                                  State                                  ###
###############################################################################

state-help:
	@echo "state subcommands"
	@echo ""
	@echo "Usage:"
	@echo "  make state-[command]"
	@echo ""
	@echo "Available Commands:"
	@echo "  dump           Run dump the state"
	@echo "  update-mainnet Update go tests with the latest mainnet state"


state: state-help

#? state-dump: Dump the state
state-dump:
	@echo "Dumping state"
	@sh ./scripts/state-dump.sh ./state

#? state-update-mainnet: Update go tests with the latest mainnet state
state-update-mainnet:
	@echo "Updating mainnet state"
	@sh ./scripts/state-dump.sh ./router/usecase/routertesting/parsing

.PHONY: state state-dump state-update-mainnet
