###############################################################################
###                                  Proto                                  ###
###############################################################################

proto-help:
	@echo "proto subcommands"
	@echo ""
	@echo "Usage:"
	@echo "  make proto-[command]"
	@echo ""
	@echo "Available Commands:"
	@echo "  all        Run proto-format and proto-gen"
	@echo "  format     Format Protobuf files"
	@echo "  gen        Generate Protobuf files"
	@echo "  image-build  Build the protobuf Docker image"
	@echo "  image-push  Push the protobuf Docker image"

proto: proto-help
proto-all: proto-format proto-gen

protoVer=0.15.1
protoImageName=ghcr.io/cosmos/proto-builder:$(protoVer)
protoImage=$(DOCKER) run --rm -v $(CURDIR):/workspace --workdir /workspace $(protoImageName)

#? proto-all: Run make proto-format proto-lint proto-gen
proto-all: proto-format proto-lint proto-gen

#? proto-gen: Generate Protobuf files
proto-gen:
	@$(protoImage) sh ./scripts/protocgen.sh
	protoc --go_out=./ --go-grpc_out=./ --proto_path=./sqsdomain/proto ./sqsdomain/proto/ingest.proto

#? proto-swagger-gen: Generate Protobuf Swagger
proto-swagger-gen:
	@echo "Generating Protobuf Swagger"
	@$(protoImage) sh ./scripts/protoc-swagger-gen.sh

#? proto-format: Format proto file
proto-format:
	@$(protoImage) find ./ -name "*.proto" -exec clang-format -i {} \;

#? proto-lint: Lint proto file
proto-lint:
	@$(protoImage) buf lint --error-format=json

#? proto-check-breaking: Check proto file is breaking
proto-check-breaking:
	@$(protoImage) buf breaking --against $(HTTPS_GIT)#branch=main

#? proto-update-deps: Update protobuf dependencies
proto-update-deps:
	@echo "Updating Protobuf dependencies"
	$(DOCKER) run --rm -v $(CURDIR)/proto:/workspace --workdir /workspace $(protoImageName) buf mod update

.PHONY: proto-all proto-gen proto-swagger-gen proto-format proto-lint proto-check-breaking proto-update-deps
