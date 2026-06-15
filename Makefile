# The name of Terraform custom provider.
CUSTOM_PROVIDER_NAME ?= terraform-provider-st-alicloud
# The url of Terraform provider.
CUSTOM_PROVIDER_URL ?= example.local/myklst/st-alicloud

.PHONY: install-local-custom-provider
install-local-custom-provider:
	HOME_DIR="$$(ls -d ~)"; \
	PLUGIN_DIR="$$HOME_DIR/.terraform.d/plugins/$(CUSTOM_PROVIDER_URL)/0.1.0/linux_amd64"; \
	mkdir -p $$PLUGIN_DIR; \
	CGO_ENABLED=0 go build -o $$PLUGIN_DIR/$(CUSTOM_PROVIDER_NAME) .
