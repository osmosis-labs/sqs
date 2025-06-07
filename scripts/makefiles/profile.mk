###############################################################################
###                                  Profile                                ###
###############################################################################

PPROF_HTTP_PORT ?= 8080
PPPROF_SECONDS ?= 60
SQS_PORT ?= 9092

profile-help:
	@echo "profile subcommands"
	@echo ""
	@echo "Usage:"
	@echo "  make profile-[command]"
	@echo ""
	@echo "Available Commands:"
	@echo "  cpu           Run CPU profiling for $(PPPROF_SECONDS) seconds"
	@echo "  heap          Run heap profiling for $(PPPROF_SECONDS) seconds"
	@echo "  block         Run block profiling for $(PPPROF_SECONDS) seconds"
	@echo "  mutex         Run mutex profiling for $(PPPROF_SECONDS) seconds"


profile: profile-cpu

#? PROFILE_URL: Generate the URL slug for the pprof profile
PROFILE_URL = $(strip $(if $(filter $(1),cpu),profile,$(1)))

#? RUN_PROFILE: Run the pprof tool with the specified profile type
define RUN_PROFILE
	go tool pprof -http=:$(PPROF_HTTP_PORT) http://localhost:$(SQS_PORT)/debug/pprof/$(call PROFILE_URL,$(1))?seconds=$(PPPROF_SECONDS)
endef

profile-%:
	$(call RUN_PROFILE,$*)

.PHONY: profile profile-help profile-%
