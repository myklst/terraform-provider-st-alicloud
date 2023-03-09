package main

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/myklst/terraform-provider-st-alicloud/alicloud"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name st-alicloud

func main() {
	testEnv := os.Getenv("PROVIDER_LOCAL_PATH")
	if testEnv != "" {
		providerserver.Serve(context.Background(), alicloud.New, providerserver.ServeOpts{
			Address: testEnv,
		})
	} else {
		providerserver.Serve(context.Background(), alicloud.New, providerserver.ServeOpts{
			Address: "registry.terraform.io/styumyum/st-alicloud",
		})
	}

}
