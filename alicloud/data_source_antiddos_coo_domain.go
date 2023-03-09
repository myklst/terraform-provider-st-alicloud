package alicloud

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudAntiddosClient "github.com/alibabacloud-go/ddoscoo-20200101/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ datasource.DataSource              = &antiddosCooDataSource{}
	_ datasource.DataSourceWithConfigure = &antiddosCooDataSource{}
)

func NewAntiddosCooDataSource() datasource.DataSource {
	return &antiddosCooDataSource{}
}

type antiddosCooDataSource struct {
	client *alicloudAntiddosClient.Client
}

type antiddosCooDataSourceModel struct {
	DomainName  types.String `tfsdk:"domain_name"`
	DomainCName types.String `tfsdk:"domain_cname"`
}

func (d *antiddosCooDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_antiddos_coo_domain"
}

func (d *antiddosCooDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the BGP-Line Anti-DDoS Instance of the current Alibaba Cloud user.",
		Attributes: map[string]schema.Attribute{
			"domain_name": schema.StringAttribute{
				Description: "Domain name of antiddos.",
				Required:    true,
			},
			"domain_cname": schema.StringAttribute{
				Description: "Domain cname of antiddos.",
				Computed:    true,
			},
		},
	}
}

func (d *antiddosCooDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(alicloudClients).antiddosClient
}

func (d *antiddosCooDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan, state antiddosCooDataSourceModel
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainName := plan.DomainName.ValueString()

	if domainName == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("domain_name"),
			"Missing Antiddos Domain Name",
			"Domain name must not be empty",
		)
		return
	}

	var antiddosCooWebRules *alicloudAntiddosClient.DescribeWebRulesResponse
	var err error
	describeWebRulesRequest := &alicloudAntiddosClient.DescribeWebRulesRequest{
		Domain:   tea.String(domainName),
		PageSize: tea.Int32(10),
	}
	runtime := &util.RuntimeOptions{}
	tryErr := func() (_e error) {
		defer func() {
			if r := tea.Recover(recover()); r != nil {
				_e = r
			}
		}()

		antiddosCooWebRules, err = d.client.DescribeWebRulesWithOptions(describeWebRulesRequest, runtime)
		if err != nil {
			return err
		}
		return nil
	}()

	if tryErr != nil {
		var error = &tea.SDKError{}
		if _t, ok := tryErr.(*tea.SDKError); ok {
			error = _t
		} else {
			error.Message = tea.String(tryErr.Error())
		}

		_, err := util.AssertAsString(error.Message)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to Query Antiddos Web Rules",
				err.Error(),
			)
			return
		}
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
