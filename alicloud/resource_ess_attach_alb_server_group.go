package alicloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudAlbClient "github.com/alibabacloud-go/alb-20200616/v2/client"
	alicloudEssClient "github.com/alibabacloud-go/ess-20220222/v2/client"
)

var (
	_ resource.Resource              = &essAttachAlbServerGroupResource{}
	_ resource.ResourceWithConfigure = &essAttachAlbServerGroupResource{}
)

func NewEssAttachAlbServerGroupResource() resource.Resource {
	return &essAttachAlbServerGroupResource{}
}

type essAttachAlbServerGroupResource struct {
	ess_client *alicloudEssClient.Client
	alb_client *alicloudAlbClient.Client
}

type essAttachAlbServerGroupModel struct {
	ScalingGroupId  types.String       `tfsdk:"scaling_group_id"`
	AlbServerGroups []*albServerGroups `tfsdk:"alb_server_groups"`
}

type albServerGroups struct {
	AlbServerGroupId types.String `tfsdk:"alb_server_group_id"`
	Weight           types.Int64  `tfsdk:"weight"`
	Port             types.Int64  `tfsdk:"port"`
}

// Metadata returns the ESS Attach ALB Server Group resource name.
func (r *essAttachAlbServerGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ess_attach_alb_server_group"
}

// Schema defines the schema for the SSL certificate binding resource.
func (r *essAttachAlbServerGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Associates an auto scaling group with a server group in Alicloud ALB.",
		Attributes: map[string]schema.Attribute{
			"scaling_group_id": schema.StringAttribute{
				Description: "Scaling Group ID.",
				Required:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"alb_server_groups": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"alb_server_group_id": schema.StringAttribute{
							Description: "ALB Server Group ID.",
							Required:    true,
						},
						"weight": schema.Int64Attribute{
							Description: "Weight for instances in ALB Server Group.",
							Required:    true,
						},
						"port": schema.Int64Attribute{
							Description: "Port for instances in ALB Server Group.",
							Required:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *essAttachAlbServerGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.ess_client = req.ProviderData.(alicloudClients).essClient
	r.alb_client = req.ProviderData.(alicloudClients).albClient
}

// Attach ALB server group with scaling groups.
func (r *essAttachAlbServerGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *essAttachAlbServerGroupModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Attach ALB server group with scaling groups
	err := r.attachServerGroup(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to attach ALB server group with scaling groups.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &essAttachAlbServerGroupModel{
		ScalingGroupId:  plan.ScalingGroupId,
		AlbServerGroups: plan.AlbServerGroups,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read the backend servers in the ALB server group.
func (r *essAttachAlbServerGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *essAttachAlbServerGroupModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	listServerGroupServersResponse, err := r.listServerGroupServers(state)
	if err != nil {
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to List servers from ALB server group.",
				err.Error(),
			)
			return
		}
	}

	var serverGroups []*albServerGroups
	var albServerGroupId string
	for _, server := range listServerGroupServersResponse.Body.Servers {
		if albServerGroupId != *server.ServerGroupId {
			serverGroups = append(serverGroups, &albServerGroups{
				AlbServerGroupId: types.StringValue(*server.ServerGroupId),
				Weight:           types.Int64Value(int64(*server.Weight)),
				Port:             types.Int64Value(int64(*server.Port)),
			})
		}
		albServerGroupId = *server.ServerGroupId
	}

	if *listServerGroupServersResponse.Body.TotalCount > 0 {
		state = &essAttachAlbServerGroupModel{
			ScalingGroupId:  types.StringValue(*listServerGroupServersResponse.Body.Servers[0].Description),
			AlbServerGroups: serverGroups,
		}
	} else {
		state = nil
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update the backend servers in ALB server group.
func (r *essAttachAlbServerGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan *essAttachAlbServerGroupModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// list servers from ALB server group.
	listServerGroupServersResponse, err := r.listServerGroupServers(plan)
	if err != nil {
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to List servers from ALB server group.",
				err.Error(),
			)
			return
		}
	}

	// Set weight for backend servers in ALB server group.
	// Retry backoff function
	setServerGroupServersWeight := func() error {
		runtime := &util.RuntimeOptions{}
		var servers []*alicloudAlbClient.UpdateServerGroupServersAttributeRequestServers

		for _, server := range listServerGroupServersResponse.Body.Servers {
			if *server.Description == plan.ScalingGroupId.ValueString() {
				for _, albServerGroups := range plan.AlbServerGroups {
					if albServerGroups.AlbServerGroupId.ValueString() == *server.ServerGroupId {
						servers = append(servers, &alicloudAlbClient.UpdateServerGroupServersAttributeRequestServers{
							ServerId:   tea.String(*server.ServerId),
							ServerType: tea.String(*server.ServerType),
							Weight:     tea.Int32(int32(albServerGroups.Weight.ValueInt64())),
							Port:       tea.Int32(int32(albServerGroups.Port.ValueInt64())),
						})
					}
				}
			}
		}

		updateServerGroupServersAttributeRequest := &alicloudAlbClient.UpdateServerGroupServersAttributeRequest{
			ServerGroupId: tea.String(plan.AlbServerGroups[0].AlbServerGroupId.ValueString()),
			Servers:       servers,
		}

		_, _err := r.alb_client.UpdateServerGroupServersAttributeWithOptions(updateServerGroupServersAttributeRequest, runtime)
		if _err != nil {
			if _t, ok := _err.(*tea.SDKError); ok {
				if isAbleToRetry(*_t.Code) {
					return _err
				} else {
					return backoff.Permanent(_err)
				}
			} else {
				return _err
			}
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(setServerGroupServersWeight, reconnectBackoff)
	if err != nil {
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to set weight for servers from ALB server group.",
				err.Error(),
			)
			return
		}
	}

	// Set state items
	state := &essAttachAlbServerGroupModel{
		ScalingGroupId:  plan.ScalingGroupId,
		AlbServerGroups: plan.AlbServerGroups,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Detach ALB server group with scaling groups.
func (r *essAttachAlbServerGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *essAttachAlbServerGroupModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Detach ALB server group with scaling groups
	err := r.detachServerGroup(state)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to detach ALB server group with scaling groups.",
			err.Error(),
		)
		return
	}
}

// Function to read the servers in alb server group.
func (r *essAttachAlbServerGroupResource) listServerGroupServers(model *essAttachAlbServerGroupModel) (*alicloudAlbClient.ListServerGroupServersResponse, error) {
	var listServerGroupServersResponse *alicloudAlbClient.ListServerGroupServersResponse
	var err error

	// Retry backoff function
	listAlbServerGroupServers := func() error {
		runtime := &util.RuntimeOptions{}

		listServerGroupServersRequest := &alicloudAlbClient.ListServerGroupServersRequest{
			ServerGroupId: tea.String(model.AlbServerGroups[0].AlbServerGroupId.ValueString()),
		}

		listServerGroupServersResponse, err = r.alb_client.ListServerGroupServersWithOptions(listServerGroupServersRequest, runtime)
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

		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(listAlbServerGroupServers, reconnectBackoff)
	if err != nil {
		return listServerGroupServersResponse, err
	}
	return listServerGroupServersResponse, nil
}

// Function to attach alb server group with scaling group.
func (r *essAttachAlbServerGroupResource) attachServerGroup(model *essAttachAlbServerGroupModel) error {
	attachAlbServerGroup := func() error {
		runtime := &util.RuntimeOptions{}
		var albServerGroups []*alicloudEssClient.AttachAlbServerGroupsRequestAlbServerGroups

		for _, albServerGroup := range model.AlbServerGroups {
			albServerGroups = append(albServerGroups,
				&alicloudEssClient.AttachAlbServerGroupsRequestAlbServerGroups{
					AlbServerGroupId: tea.String(albServerGroup.AlbServerGroupId.ValueString()),
					Weight:           tea.Int32(int32(albServerGroup.Weight.ValueInt64())),
					Port:             tea.Int32(int32(albServerGroup.Port.ValueInt64())),
				},
			)
		}

		attachAlbServerGroupsRequest := &alicloudEssClient.AttachAlbServerGroupsRequest{
			RegionId:        r.ess_client.RegionId,
			ScalingGroupId:  tea.String(model.ScalingGroupId.ValueString()),
			AlbServerGroups: albServerGroups,
			ForceAttach:     tea.Bool(true),
		}

		_, _err := r.ess_client.AttachAlbServerGroupsWithOptions(attachAlbServerGroupsRequest, runtime)
		if _err != nil {
			if _t, ok := _err.(*tea.SDKError); ok {
				if isAbleToRetry(*_t.Code) {
					return _err
				} else {
					return backoff.Permanent(_err)
				}
			} else {
				return _err
			}
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(attachAlbServerGroup, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}

// Function to dettach alb server group with scaling group.
func (r *essAttachAlbServerGroupResource) detachServerGroup(model *essAttachAlbServerGroupModel) error {
	detachAlbServerGroup := func() error {
		runtime := &util.RuntimeOptions{}
		var albServerGroups []*alicloudEssClient.DetachAlbServerGroupsRequestAlbServerGroups

		for _, albServerGroup := range model.AlbServerGroups {
			albServerGroups = append(albServerGroups,
				&alicloudEssClient.DetachAlbServerGroupsRequestAlbServerGroups{
					AlbServerGroupId: tea.String(albServerGroup.AlbServerGroupId.ValueString()),
					Port:             tea.Int32(int32(albServerGroup.Port.ValueInt64())),
				},
			)
		}

		detachAlbServerGroupsRequest := &alicloudEssClient.DetachAlbServerGroupsRequest{
			RegionId:        r.ess_client.RegionId,
			ScalingGroupId:  tea.String(model.ScalingGroupId.ValueString()),
			AlbServerGroups: albServerGroups,
			ForceDetach:     tea.Bool(true),
		}

		_, _err := r.ess_client.DetachAlbServerGroupsWithOptions(detachAlbServerGroupsRequest, runtime)
		if _err != nil {
			if _t, ok := _err.(*tea.SDKError); ok {
				if isAbleToRetry(*_t.Code) {
					return _err
				} else {
					return backoff.Permanent(_err)
				}
			} else {
				return _err
			}
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(detachAlbServerGroup, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}
