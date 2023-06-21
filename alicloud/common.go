package alicloud

import "github.com/hashicorp/terraform-plugin-framework/types"

type clientConfig struct {
	Region    types.String `tfsdk:"region"`
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
}

type clientConfigWithZone struct {
	Region    types.String `tfsdk:"region"`
	Zone      types.String `tfsdk:"zone"`
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
}

func (cfg *clientConfigWithZone) getClientConfig() *clientConfig {
	return &clientConfig{
		Region:    cfg.Region,
		AccessKey: cfg.AccessKey,
		SecretKey: cfg.SecretKey,
	}
}
