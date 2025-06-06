###############################################################################
###                                  Profile                                ###
###############################################################################

profile-help:
	@echo "profile subcommands"
	@echo ""
	@echo "Usage:"
	@echo "  make profile-[command]"
	@echo ""
	@echo "Available Commands:"
	@echo "  cpu           Run CPU profiling for 60 seconds"
	@echo "  heap          Run heap profiling for 60 seconds"
	@echo "  block         Run block profiling for 60 seconds"


profile: profile-cpu

profile-cpu:
	go tool pprof -http=:8080 http://localhost:9092/debug/pprof/profile?seconds=60

profile-heap:
	go tool pprof -http=:8080 http://localhost:9092/debug/pprof/heap?seconds=60

profile-block:
	go tool pprof -http=:8080 http://localhost:9092/debug/pprof/block?seconds=60

.PHONY: profile-unit
