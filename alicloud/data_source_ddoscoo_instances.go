package alicloud

import (
	"context"
	"regexp"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudAntiddosClient "github.com/alibabacloud-go/ddoscoo-20200101/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ datasource.DataSource              = &ddoscooInstancesDataSource{}
	_ datasource.DataSourceWithConfigure = &ddoscooInstancesDataSource{}
)

func NewDdosCooInstancesDataSource() datasource.DataSource {
	return &ddoscooInstancesDataSource{}
}

type ddoscooInstancesDataSource struct {
	client *alicloudAntiddosClient.Client
}

type ddoscooInstancesDataSourceModel struct {
	Remark    types.String           `tfsdk:"remark_regex"`
	IDs       types.List             `tfsdk:"ids"`
	Instances []*antiddosCooInstance `tfsdk:"instances"`
}

type antiddosCooInstance struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	BaseBandwidth    types.Int64  `tfsdk:"base_bandwidth"`
	Bandwidth        types.Int64  `tfsdk:"bandwidth"`
	ServiceBandwidth types.Int64  `tfsdk:"service_bandwidth"`
	PortCount        types.Int64  `tfsdk:"port_count"`
	DomainCount      types.Int64  `tfsdk:"domain_count"`
	Remark           types.String `tfsdk:"remark"`
	IpMode           types.String `tfsdk:"ip_mode"`
	DebtStatus       types.Int64  `tfsdk:"debt_status"`
	Edition          types.Int64  `tfsdk:"edition"`
	IpVersion        types.String `tfsdk:"ip_version"`
	Status           types.Int64  `tfsdk:"status"`
	Enabled          types.Int64  `tfsdk:"enabled"`
	ExpireTime       types.Int64  `tfsdk:"expire_time"`
	CreateTime       types.Int64  `tfsdk:"create_time"`
	Eip              types.List   `tfsdk:"eip"`
}

func (d *ddoscooInstancesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ddoscoo_instances"
}

func (d *ddoscooInstancesDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the AntiDDoS domain resources of the current AliCloud user.",
		Attributes: map[string]schema.Attribute{
			"remark_regex": schema.StringAttribute{
				Description: "Remark of AntiDDoS instances.",
				Optional:    true,
			},
			"ids": schema.ListAttribute{
				Description: "List of IDs of AntiDDoS instance.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"instances": schema.ListNestedAttribute{
				Description: "A list of Anti-DDoS instances",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "ID of AntiDDoS instance.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "Name of AntiDDoS instance.",
							Computed:    true,
						},
						"base_bandwidth": schema.Int64Attribute{
							Description: "Base bandwidth of AntiDDoS instance.",
							Computed:    true,
						},
						"bandwidth": schema.Int64Attribute{
							Description: "Bandwidth of AntiDDoS instacne.",
							Computed:    true,
						},
						"service_bandwidth": schema.Int64Attribute{
							Description: "Service bandwidth of AntiDDoS instance.",
							Computed:    true,
						},
						"port_count": schema.Int64Attribute{
							Description: "Port count of AntiDDoS instance.",
							Computed:    true,
						},
						"domain_count": schema.Int64Attribute{
							Description: "Domain count of AntiDDoS instance.",
							Computed:    true,
						},
						"remark": schema.StringAttribute{
							Description: "Remark of AntiDDoS instance.",
							Computed:    true,
						},
						"ip_mode": schema.StringAttribute{
							Description: "IP Mode of AntiDDoS instance.",
							Computed:    true,
						},
						"debt_status": schema.Int64Attribute{
							Description: "Debt status of AntiDDoS instance.",
							Computed:    true,
						},
						"edition": schema.Int64Attribute{
							Description: "Edition of AntiDDoS instance.",
							Computed:    true,
						},
						"ip_version": schema.StringAttribute{
							Description: "IP version of AntiDDoS instance.",
							Computed:    true,
						},
						"status": schema.Int64Attribute{
							Description: "Status of AntiDDoS instance.",
							Computed:    true,
						},
						"enabled": schema.Int64Attribute{
							Description: "If the AntiDDoS instance is enabled",
							Computed:    true,
						},
						"expire_time": schema.Int64Attribute{
							Description: "Expire time of AntiDDoS instance.",
							Computed:    true,
						},
						"create_time": schema.Int64Attribute{
							Description: "Create time of AntiDDoS instance.",
							Computed:    true,
						},
						"eip": schema.ListAttribute{
							Description: "EIP of AntiDDoS instance.",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (d *ddoscooInstancesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(alicloudClients).antiddosClient
}

func (d *ddoscooInstancesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan, state ddoscooInstancesDataSourceModel
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.IDs = plan.IDs
	state.Remark = plan.Remark
	state.Instances = []*antiddosCooInstance{}

	describeInstancesRequest := &alicloudAntiddosClient.DescribeInstancesRequest{
		PageNumber: tea.String("1"),
		PageSize:   tea.String("20"),
	}
	describeInstanceSpecsRequest := &alicloudAntiddosClient.DescribeInstanceSpecsRequest{}
	describeInstanceDetailsRequest := &alicloudAntiddosClient.DescribeInstanceDetailsRequest{}

	var nameRegex *regexp.Regexp

	// Describe Instances
	readInstances := func() error {
		runtime := &util.RuntimeOptions{}

		if !(plan.IDs.IsNull() || plan.IDs.IsUnknown()) {
			// Plan ID List to List of String
			var planIdsList []string
			for _, x := range plan.IDs.Elements() {
				planIdsList = append(planIdsList, trimStringQuotes(x.String()))
			}

			describeInstancesRequest.InstanceIds = tea.StringSlice(planIdsList)
		}

		if !(plan.Remark.IsNull() || plan.Remark.IsUnknown()) {
			// Convert Remark Input to Regex
			v := plan.Remark.ValueString()
			if r, err := regexp.Compile(v); err != nil {
				resp.Diagnostics.AddError(
					"[API ERROR] Failed to Convert Remark Input to Regex",
					err.Error(),
				)
			} else {
				nameRegex = r
			}
		}

		// Describe Instances List
		antiddosInstances, err := d.client.DescribeInstancesWithOptions(describeInstancesRequest, runtime)
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

		var antiddosInstancesList []string
		if *antiddosInstances.Body.TotalCount > int64(0) {

			for _, instances := range antiddosInstances.Body.Instances {
				// Regex Filter
				if nameRegex != nil && !(nameRegex.MatchString(*instances.Remark)) {
					continue
				}
				antiddosInstancesList = append(antiddosInstancesList, *instances.InstanceId)
			}

			// Describe Instance Specs
			describeInstanceSpecsRequest.InstanceIds = tea.StringSlice(antiddosInstancesList)
			antiddosInstanceSpecs, err := d.client.DescribeInstanceSpecsWithOptions(describeInstanceSpecsRequest, runtime)
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

			// Describe Instance Details
			describeInstanceDetailsRequest.InstanceIds = tea.StringSlice(antiddosInstancesList)
			antiddosInstanceDetails, err := d.client.DescribeInstanceDetailsWithOptions(describeInstanceDetailsRequest, runtime)
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

			// Assign all values into instances
			for i := 0; i < len(antiddosInstancesList); i++ {

				var instanceEipList []attr.Value
				for _, instanceDetailsEip := range antiddosInstanceDetails.Body.InstanceDetails[i].EipInfos {
					instanceEipList = append(instanceEipList, types.StringValue(*instanceDetailsEip.Eip))
				}

				instanceDetail := &antiddosCooInstance{
					ID:               types.StringValue(antiddosInstancesList[i]),
					Name:             types.StringValue(*antiddosInstances.Body.Instances[i].Remark),
					BaseBandwidth:    types.Int64Value(int64(*antiddosInstanceSpecs.Body.InstanceSpecs[i].BaseBandwidth)),
					Bandwidth:        types.Int64Value(int64(*antiddosInstanceSpecs.Body.InstanceSpecs[i].ElasticBandwidth)),
					ServiceBandwidth: types.Int64Value(int64(*antiddosInstanceSpecs.Body.InstanceSpecs[i].BandwidthMbps)),
					PortCount:        types.Int64Value(int64(*antiddosInstanceSpecs.Body.InstanceSpecs[i].PortLimit)),
					DomainCount:      types.Int64Value(int64(*antiddosInstanceSpecs.Body.InstanceSpecs[i].DomainLimit)),
					Remark:           types.StringValue(*antiddosInstances.Body.Instances[i].Remark),
					IpMode:           types.StringValue(*antiddosInstances.Body.Instances[i].IpMode),
					DebtStatus:       types.Int64Value(int64(*antiddosInstances.Body.Instances[i].DebtStatus)),
					Edition:          types.Int64Value(int64(*antiddosInstances.Body.Instances[i].Edition)),
					IpVersion:        types.StringValue(*antiddosInstances.Body.Instances[i].IpVersion),
					Status:           types.Int64Value(int64(*antiddosInstances.Body.Instances[i].Status)),
					Enabled:          types.Int64Value(int64(*antiddosInstances.Body.Instances[i].Enabled)),
					ExpireTime:       types.Int64Value(*antiddosInstances.Body.Instances[i].ExpireTime),
					CreateTime:       types.Int64Value(*antiddosInstances.Body.Instances[i].CreateTime),
					Eip:              types.ListValueMust(types.StringType, instanceEipList),
				}
				state.Instances = append(state.Instances, instanceDetail)
			}
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 60 * time.Second

	err := backoff.Retry(readInstances, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read Anti-DDoS Instances",
			err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
