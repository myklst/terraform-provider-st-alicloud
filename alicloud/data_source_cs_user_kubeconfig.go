package alicloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudCsClient "github.com/alibabacloud-go/cs-20151215/v4/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ datasource.DataSource              = &csUserKubeconfigDataSource{}
	_ datasource.DataSourceWithConfigure = &csUserKubeconfigDataSource{}
)

func NewCsUserKubeconfigDataSource() datasource.DataSource {
	return &csUserKubeconfigDataSource{}
}

type csUserKubeconfigDataSource struct {
	client *alicloudCsClient.Client
}

type csUserKubeconfigDataSourceModel struct {
	ClientConfig *clientConfig `tfsdk:"client_config"`
	ClusterId    types.String  `tfsdk:"cluster_id"`
	Kubeconfig   types.String  `tfsdk:"kubeconfig"`
}

func (d *csUserKubeconfigDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cs_user_kubeconfig"
}

func (d *csUserKubeconfigDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the Kubeconfig of container service for the set Alibaba Cloud user.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Description: "Cluster ID of container service for Kubernetes.",
				Required:    true,
			},
			"kubeconfig": schema.StringAttribute{
				Description: "Kubeconfig of container service for Kubernetes.",
				Computed:    true,
				Sensitive:   true,
			},
		},
		Blocks: map[string]schema.Block{
			"client_config": schema.SingleNestedBlock{
				Description: "Config to override default client created in Provider. " +
					"This block will not be recorded in state file.",
				Attributes: map[string]schema.Attribute{
					"region": schema.StringAttribute{
						Description: "The region of the Container Service for Kubernetes. Default to " +
							"use region configured in the provider.",
						Optional: true,
					},
					"access_key": schema.StringAttribute{
						Description: "The access key for user to query Kubeconfig. " +
							"Default to use access key configured in " +
							"the provider.",
						Optional: true,
					},
					"secret_key": schema.StringAttribute{
						Description: "The secret key for user to query Kubeconfig. " +
							"Default to use secret key configured in " +
							"the provider.",
						Optional: true,
					},
				},
			},
		},
	}
}

func (d *csUserKubeconfigDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(alicloudClients).csClient
}

func (d *csUserKubeconfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan, state csUserKubeconfigDataSourceModel
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.ClientConfig == nil {
		plan.ClientConfig = &clientConfig{}
	}

	initClient, clientCredentialsConfig, initClientDiags := initNewClient(&d.client.Client, plan.ClientConfig)
	if initClientDiags.HasError() {
		resp.Diagnostics.Append(initClientDiags...)
		return
	}

	if initClient {
		var err error
		d.client, err = alicloudCsClient.NewClient(clientCredentialsConfig)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Reinitialize AliCloud CS API Client",
				"An unexpected error occurred when creating the AliCloud CS API client. "+
					"If the error is not clear, please contact the provider developers.\n\n"+
					"AliCloud CS Client Error: "+err.Error(),
			)
			return
		}
	}

	if plan.ClusterId.IsNull() || plan.ClusterId.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("cluster_id"),
			"Missing Cluster ID",
			"Cluster ID must not be empty.",
		)
		return
	}

	describeClusterUserKubeconfigRequest := &alicloudCsClient.DescribeClusterUserKubeconfigRequest{}

	var userKubeconfig *alicloudCsClient.DescribeClusterUserKubeconfigResponse
	var err error

	describeUserKubeconfig := func() (err error) {
		runtime := &util.RuntimeOptions{}
		headers := make(map[string]*string)

		userKubeconfig, err = d.client.DescribeClusterUserKubeconfigWithOptions(tea.String(plan.ClusterId.ValueString()), describeClusterUserKubeconfigRequest, headers, runtime)
		if err != nil {
			if _t, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*_t.Code) {
					return err
				} else {
					return backoff.Permanent(err)
				}
			} else {
				return err
			}
		}
		return
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(describeUserKubeconfig, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe Container Service User Kubeconfig",
			err.Error(),
		)
		return
	}

	if userKubeconfig.String() != "{}" {
		state.ClusterId = plan.ClusterId
		state.Kubeconfig = types.StringValue(*userKubeconfig.Body.Config)
	} else {
		resp.State.RemoveResource(ctx)
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
