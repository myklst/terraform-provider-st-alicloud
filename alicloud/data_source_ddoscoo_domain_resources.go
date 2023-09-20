package alicloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudAntiddosClient "github.com/alibabacloud-go/ddoscoo-20200101/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ datasource.DataSource              = &ddoscooDomainResourcesDataSource{}
	_ datasource.DataSourceWithConfigure = &ddoscooDomainResourcesDataSource{}
)

func NewDdosCooDomainResourcesDataSource() datasource.DataSource {
	return &ddoscooDomainResourcesDataSource{}
}

type ddoscooDomainResourcesDataSource struct {
	client *alicloudAntiddosClient.Client
}

type ddoscooDomainResourcesDataSourceModel struct {
	ClientConfig *clientConfig `tfsdk:"client_config"`
	DomainName   types.String  `tfsdk:"domain_name"`
	DomainCName  types.String  `tfsdk:"domain_cname"`
}

func (d *ddoscooDomainResourcesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ddoscoo_domain_resources"
}

func (d *ddoscooDomainResourcesDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the AntiDDoS domain resources of the current AliCloud user.",
		Attributes: map[string]schema.Attribute{
			"domain_name": schema.StringAttribute{
				Description: "Domain name of AntiDDoS.",
				Required:    true,
			},
			"domain_cname": schema.StringAttribute{
				Description: "Domain CNAME of AntiDDoS.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"client_config": schema.SingleNestedBlock{
				Description: "Config to override default client created in Provider. " +
					"This block will not be recorded in state file.",
				Attributes: map[string]schema.Attribute{
					"region": schema.StringAttribute{
						Description: "The region of the AntiDDoS. Default to " +
							"use region configured in the provider.",
						Optional: true,
					},
					"access_key": schema.StringAttribute{
						Description: "The access key that have permissions to list " +
							"AntiDDoS domain resources. Default to use access key " +
							"configured in the provider.",
						Optional: true,
					},
					"secret_key": schema.StringAttribute{
						Description: "The secret key that have permissions to lsit " +
							"AntiDDoS domain resources. Default to use secret key " +
							"configured in the provider.",
						Optional: true,
					},
				},
			},
		},
	}
}

func (d *ddoscooDomainResourcesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(alicloudClients).antiddosClient
}

func (d *ddoscooDomainResourcesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan, state ddoscooDomainResourcesDataSourceModel
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
		d.client, err = alicloudAntiddosClient.NewClient(clientCredentialsConfig)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Reinitialize AliCloud AntiDDoS API Client",
				"An unexpected error occurred when creating the AliCloud AntiDDoS "+
					"API client. If the error is not clear, please contact the provider "+
					"developers.\n\nAliCloud AntiDDoS Client Error: "+err.Error(),
			)
			return
		}
	}

	domainName := plan.DomainName.ValueString()

	if domainName == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("domain_name"),
			"Missing AntiDDoS domain name",
			"Domain name must not be empty",
		)
		return
	}

	describeWebRulesRequest := &alicloudAntiddosClient.DescribeWebRulesRequest{
		Domain:   tea.String(domainName),
		PageSize: tea.Int32(10),
	}

	var antiddosCooWebRules *alicloudAntiddosClient.DescribeWebRulesResponse
	var err error
	describeWebRules := func() (err error) {
		runtime := &util.RuntimeOptions{}

		antiddosCooWebRules, err = d.client.DescribeWebRulesWithOptions(describeWebRulesRequest, runtime)
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

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(describeWebRules, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe Antiddos Web Rule.",
			err.Error(),
		)
		return
	}

	if antiddosCooWebRules.String() != "{}" && *antiddosCooWebRules.Body.TotalCount > int64(0) {
		state.DomainName = types.StringValue(*antiddosCooWebRules.Body.WebRules[0].Domain)
		state.DomainCName = types.StringValue(*antiddosCooWebRules.Body.WebRules[0].Cname)
	} else {
		state.DomainName = types.StringNull()
		state.DomainCName = types.StringNull()
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
