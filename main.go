package main

import (
	"context"
	"os"

	"github.com/myklst/terraform-provider-st-alicloud/alicloud"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name st-alicloud

func main() {
	testEnv := os.Getenv("ST-ALICLOUD_LOCAL_PATH")
	if testEnv != "" {
		providerserver.Serve(context.Background(), alicloud.New, providerserver.ServeOpts{
			Address: "myklst/st-alicloud",
		})
	} else {
		providerserver.Serve(context.Background(), alicloud.New, providerserver.ServeOpts{
			Address: testEnv,
		})
	}

}
