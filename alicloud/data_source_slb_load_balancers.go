package alicloud

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	alicloudOpenapiClient "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	alicloudSlbClient "github.com/alibabacloud-go/slb-20140515/v4/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &slbLoadBalancersDataSource{}
	_ datasource.DataSourceWithConfigure = &slbLoadBalancersDataSource{}
)

func NewSlbLoadBalancersDataSource() datasource.DataSource {
	return &slbLoadBalancersDataSource{}
}

type slbLoadBalancersDataSource struct {
	defaultCredentialConfig *alicloudOpenapiClient.Config
}

type slbLoadBalancersDataSourceModel struct {
	Region        types.String              `tfsdk:"region"`
	AccessKey     types.String              `tfsdk:"access_key"`
	SecretKey     types.String              `tfsdk:"secret_key"`
	Name          types.String              `tfsdk:"name"`
	Tags          types.Map                 `tfsdk:"tags"`
	LoadBalancers []*slbLoadBalancersDetail `tfsdk:"load_balancers"`
}

type slbLoadBalancersDetail struct {
	Id   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Tags types.Map    `tfsdk:"tags"`
}

func (d *slbLoadBalancersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_slb_load_balancers"
}

func (d *slbLoadBalancersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the Server Load Balancers in desired region or user account.",
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Description: "The region of the SLBs. Default to use region configured" +
					"in the provider.",
				Optional: true,
			},
			"access_key": schema.StringAttribute{
				Description: "The access key that have permissions to list SLBs. " +
					"Default to use access key configured in the provider.",
				Optional: true,
			},
			"secret_key": schema.StringAttribute{
				Description: "The secret key that have permissions to lsit SLBs. " +
					"Default to use secret key configured in the provider.",
				Optional: true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the SLBs.",
				Optional:    true,
			},
			"tags": schema.MapAttribute{
				Description: "A map of tags assigned to the SLB instances.",
				ElementType: types.StringType,
				Optional:    true,
			},
			"load_balancers": schema.ListNestedAttribute{
				Description: "A list of SLBs.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "ID of the SLB.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the SLB.",
							Computed:    true,
						},
						"tags": schema.MapAttribute{
							Description: "The tags of the SLB.",
							ElementType: types.StringType,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *slbLoadBalancersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.defaultCredentialConfig = req.ProviderData.(alicloudClients).clientCredentialsConfig
}

func (d *slbLoadBalancersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan *slbLoadBalancersDataSourceModel

	getPlanDiags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if getPlanDiags.HasError() {
		return
	}

	pageNumber := 0

	var region, accessKey, secretKey string

	if plan.Region.IsUnknown() || plan.Region.IsNull() || plan.Region.String() == "" {
		region = *d.defaultCredentialConfig.RegionId
	} else {
		region = plan.Region.ValueString()
	}

	if plan.AccessKey.IsUnknown() || plan.AccessKey.IsNull() || plan.AccessKey.String() == "" {
		accessKey = *d.defaultCredentialConfig.AccessKeyId
	} else {
		accessKey = plan.AccessKey.ValueString()
	}

	if plan.SecretKey.IsUnknown() || plan.SecretKey.IsNull() || plan.SecretKey.String() == "" {
		secretKey = *d.defaultCredentialConfig.AccessKeySecret
	} else {
		secretKey = plan.SecretKey.ValueString()
	}

	clientCredentialsConfig := &alicloudOpenapiClient.Config{
		RegionId:        &region,
		AccessKeyId:     &accessKey,
		AccessKeySecret: &secretKey,
	}

	// AliCloud SLB Client
	slbClientConfig := clientCredentialsConfig
	slbClientConfig.Endpoint = tea.String(fmt.Sprintf("slb.%s.aliyuncs.com", region))
	slbClient, err := alicloudSlbClient.NewClient(slbClientConfig)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create AliCloud SLB API Client",
			"An unexpected error occurred when creating the AliCloud SLB API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"AliCloud SLB Client Error: "+err.Error(),
		)
		return
	}

	state := &slbLoadBalancersDataSourceModel{}
	state.LoadBalancers = []*slbLoadBalancersDetail{}

	describeLoadBalancersRequest := &alicloudSlbClient.DescribeLoadBalancersRequest{
		RegionId: tea.String(*slbClient.RegionId),
		PageSize: tea.Int32(100),
	}

	if !(plan.Name.IsUnknown() && plan.Name.IsNull()) {
		state.Name = plan.Name
		describeLoadBalancersRequest.LoadBalancerName = tea.String(plan.Name.ValueString())
	}

	inputTags := make(map[string]string)
	if !(plan.Tags.IsUnknown() && plan.Tags.IsNull()) {
		state.Tags = plan.Tags
		// Convert from Terraform map type to Go map type
		convertTagsDiags := plan.Tags.ElementsAs(ctx, &inputTags, false)
		resp.Diagnostics.Append(convertTagsDiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Construct the AliCloud tag struct.
		slbTags := make([]*alicloudSlbClient.DescribeLoadBalancersResponseBodyLoadBalancersLoadBalancerTagsTag, 0)
		for key, value := range inputTags {
			if key == "app" {
				slbTags = append(slbTags, &alicloudSlbClient.DescribeLoadBalancersResponseBodyLoadBalancersLoadBalancerTagsTag{
					TagKey:   tea.String(key),
					TagValue: tea.String(value),
				})
			}
		}

		// Convert the tag struct to JSON string that will be used for DescribeLoadBalancersWithOptions in AliCloud API client.
		if len(slbTags) != 0 {
			jsonTags, err := json.Marshal(slbTags)
			if err != nil {
				resp.Diagnostics.AddError(
					"[ERROR] failed to marshal tag",
					err.Error(),
				)
				return
			}

			describeLoadBalancersRequest.Tags = tea.String(string(jsonTags))
		}
	}
	runtime := &util.RuntimeOptions{}

	for {
		pageNumber++
		describeLoadBalancersRequest.PageNumber = tea.Int32(int32(pageNumber))

		describeLoadBalancersResponse, err := slbClient.DescribeLoadBalancersWithOptions(describeLoadBalancersRequest, runtime)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] failed to query load balancers",
				err.Error(),
			)
			return
		}

	slbLoop:
		for _, loadBalancer := range describeLoadBalancersResponse.Body.LoadBalancers.LoadBalancer {
			if len(loadBalancer.Tags.Tag) < 1 {
				continue
			} else {
				tags := make(map[string]attr.Value)

				// Convert AliCloud tag format to map[string]string
				slbTagQuried := make(map[string]string)
				for _, tag := range loadBalancer.Tags.Tag {
					slbTagQuried[*tag.TagKey] = *tag.TagValue
					tags[*tag.TagKey] = types.StringValue(*tag.TagValue)
				}

				// Search whether the quried slb contains the input tags
				for inputTagKey, inputTagValue := range inputTags {
					// Check whether the load balance have the tag key, break and loop next load balance
					// if key not found.
					value, ok := slbTagQuried[inputTagKey]
					if ok {
						// '|' is assumed as string delimiter, split them to a list of string
						// and compare with the input tag value, break if none of it are matched
						if strings.Contains(value, "|") {
							matched := false
							tagList := strings.Split(value, "|")
							for _, t := range tagList {
								if t == inputTagValue {
									matched = true
								}
							}
							if !matched {
								continue slbLoop
							}
						// Compare with the input tag value, break if not matched
						} else if value != inputTagValue {
							continue slbLoop
						}
					} else {
						continue slbLoop
					}
				}

				slbDetail := &slbLoadBalancersDetail{
					Id:   types.StringValue(*loadBalancer.LoadBalancerId),
					Name: types.StringValue(*loadBalancer.LoadBalancerName),
					Tags: types.MapValueMust(types.StringType, tags),
				}
				state.LoadBalancers = append(state.LoadBalancers, slbDetail)
			}
		}

		// Stop entering to second page if any result is found.
		if len(state.LoadBalancers) > 0 {
			break
		}

		// If page number * page size is larger or equal to the total count, then that mean it's the last page.
		if *describeLoadBalancersResponse.Body.PageNumber**describeLoadBalancersResponse.Body.PageSize >= *describeLoadBalancersResponse.Body.TotalCount {
			break
		}
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
