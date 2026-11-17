# This makefile should be used to hold functions/variables

define github_url
    https://github.com/$(GITHUB)/releases/download/v$(VERSION)/$(ARCHIVE)
endef

# creates a directory bin.
bin:
	@ mkdir -p $@

# ~~~ Tools ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

# ~~ [ mockery ] ~~~ https://github.com/vektra/mockery ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

MOCKERY := $(shell command -v mockery || echo "bin/mockery")
mockery: bin/mockery ## Installs mockery (mocks generation)

bin/mockery: VERSION := 2.35.3
bin/mockery: GITHUB  := vektra/mockery
# The release archives are named by architecture: x86_64 or arm64 (aarch64 on Linux).
bin/mockery: ARCH    := $(shell uname -m | sed 's/aarch64/arm64/')
bin/mockery: ARCHIVE := mockery_$(VERSION)_$(OSTYPE)_$(ARCH).tar.gz
bin/mockery: bin
	@ printf "Install mockery... "
	@ curl -Ls $(call github_url) | tar -zOxf -  mockery > $@ && chmod +x $@
	@ echo "done."