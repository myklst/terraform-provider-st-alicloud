package alicloud

import "github.com/hashicorp/terraform-plugin-framework/types"

type clientConfig struct {
	Region    types.String `tfsdk:"region"`
	Zone      types.String `tfsdk:"zone"`
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
}
