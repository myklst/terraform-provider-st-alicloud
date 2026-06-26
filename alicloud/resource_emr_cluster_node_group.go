// alicloud/resource_alicloud_emr_cluster_node_group.go
package alicloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudEmrClient "github.com/alibabacloud-go/emr-20210320/v3/client"
)

var (
	_ resource.Resource                = &emrClusterNodeGroupResource{}
	_ resource.ResourceWithConfigure   = &emrClusterNodeGroupResource{}
	_ resource.ResourceWithImportState = &emrClusterNodeGroupResource{}
)

func NewEmrClusterNodeGroupResource() resource.Resource {
	return &emrClusterNodeGroupResource{}
}

type emrClusterNodeGroupResource struct {
	client *alicloudEmrClient.Client
}

type emrClusterNodeGroupModel struct {
	ClusterId          types.String             `tfsdk:"cluster_id"`
	NodeGroupId        types.String             `tfsdk:"node_group_id"`
	NodeGroupName      types.String             `tfsdk:"node_group_name"`
	NodeGroupType      types.String             `tfsdk:"node_group_type"`
	InstanceTypes      types.List               `tfsdk:"instance_types"`
	VSwitchIds         types.List               `tfsdk:"vswitch_ids"`
	NodeCount          types.Int64              `tfsdk:"node_count"`
	SystemDisk         *diskModel               `tfsdk:"system_disk"`
	DataDisks          []*diskModel             `tfsdk:"data_disks"`
	SpotStrategy       types.String             `tfsdk:"spot_strategy"`
	WithPublicIp       types.Bool               `tfsdk:"with_public_ip"`
	PaymentType        types.String             `tfsdk:"payment_type"`
	GracefulShutdown   types.Bool               `tfsdk:"graceful_shutdown"`
	SpotInstanceRemedy types.Bool               `tfsdk:"spot_instance_remedy"`
	SubscriptionConfig *subscriptionConfigModel `tfsdk:"subscription_config"`
}

type diskModel struct {
	Category types.String `tfsdk:"category"`
	Size     types.Int64  `tfsdk:"size"`
	Count    types.Int64  `tfsdk:"count"`
}

type subscriptionConfigModel struct {
	AutoRenew             types.Bool   `tfsdk:"auto_renew"`
	AutoRenewDuration     types.Int64  `tfsdk:"auto_renew_duration"`
	AutoRenewDurationUnit types.String `tfsdk:"auto_renew_duration_unit"`
	PaymentDuration       types.Int64  `tfsdk:"payment_duration"`
	PaymentDurationUnit   types.String `tfsdk:"payment_duration_unit"`
}

func (r *emrClusterNodeGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_emr_cluster_node_group"
}

func (r *emrClusterNodeGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an ECS-based node group (TASK group, supports auto scaling) within an existing AliCloud E-MapReduce (EMR) cluster. Use this resource because alicloud_emrv2_cluster does not expose auto scaling configuration for node groups.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the EMR cluster (emrv2) this node group belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"node_group_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the node group, assigned by AliCloud.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"node_group_name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the node group.",
			},
			"node_group_type": schema.StringAttribute{
				Required:    true,
				Description: "Type of the node group. Only TASK node groups support auto scaling.",
				Validators: []validator.String{
					stringvalidator.OneOf("MASTER", "CORE", "TASK"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"instance_types": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "List of ECS instance types eligible for this node group, in priority order.",
			},
			"vswitch_ids": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "VSwitch IDs the node group's ECS instances can be created in.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"node_count": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Desired number of nodes. Ignored once auto_scaling_policy.enable is true; AliCloud manages the count between min_capacity and max_capacity.",
			},
			"spot_strategy": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Spot strategy for the ECS instances. Valid values: NoSpot, SpotWithPriceLimit, SpotAsPriceGo.",
				Validators: []validator.String{
					stringvalidator.OneOf("NoSpot", "SpotWithPriceLimit", "SpotAsPriceGo"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"with_public_ip": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether to assign a public IP to nodes in this group.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"payment_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Payment type for the node group. Valid values: PayAsYouGo, Subscription. Defaults to the cluster payment type when not set.",
				Validators: []validator.String{
					stringvalidator.OneOf("PayAsYouGo", "Subscription"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"graceful_shutdown": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether to enable graceful decommission for components deployed on this node group.",
			},
			"spot_instance_remedy": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether to replace preempted spot instances automatically when a reclaim notice is received.",
			},
			"subscription_config": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Subscription (prepaid) configuration for the node group. Only valid when payment_type is Subscription.",
				Attributes: map[string]schema.Attribute{
					"auto_renew": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Whether to enable auto-renewal.",
					},
					"auto_renew_duration": schema.Int64Attribute{
						Optional:    true,
						Computed:    true,
						Description: "Auto-renewal duration. Takes effect when auto_renew is true.",
					},
					"auto_renew_duration_unit": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Unit for auto_renew_duration. Valid value: Month.",
						Validators: []validator.String{
							stringvalidator.OneOf("Month"),
						},
					},
					"payment_duration": schema.Int64Attribute{
						Required:    true,
						Description: "Subscription payment duration.",
					},
					"payment_duration_unit": schema.StringAttribute{
						Required:    true,
						Description: "Unit for payment_duration. Valid value: Month.",
						Validators: []validator.String{
							stringvalidator.OneOf("Month"),
						},
					},
				},
			},
			"system_disk": schema.SingleNestedAttribute{
				Required:    true,
				Description: "System disk configuration for nodes in this group.",
				Attributes: map[string]schema.Attribute{
					"category": schema.StringAttribute{
						Required: true,
					},
					"size": schema.Int64Attribute{
						Required: true,
						Validators: []validator.Int64{
							int64validator.AtLeast(40),
						},
					},
					"count": schema.Int64Attribute{
						Optional: true,
						Computed: true,
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"data_disks": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Data disk configuration for nodes in this group.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"category": schema.StringAttribute{
							Required: true,
						},
						"size": schema.Int64Attribute{
							Required: true,
						},
						"count": schema.Int64Attribute{
							Optional: true,
							Computed: true,
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *emrClusterNodeGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).emrClient
}

func (r *emrClusterNodeGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *emrClusterNodeGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var instanceTypes, vswitchIds []*string
	resp.Diagnostics.Append(plan.InstanceTypes.ElementsAs(ctx, &instanceTypes, false)...)
	resp.Diagnostics.Append(plan.VSwitchIds.ElementsAs(ctx, &vswitchIds, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &alicloudEmrClient.CreateNodeGroupRequest{
		RegionId:  tea.String(*r.client.Endpoint),
		ClusterId: tea.String(plan.ClusterId.ValueString()),
		NodeGroup: &alicloudEmrClient.NodeGroupConfig{
			NodeGroupName: tea.String(plan.NodeGroupName.ValueString()),
			NodeGroupType: tea.String(plan.NodeGroupType.ValueString()),
			InstanceTypes: instanceTypes,
			VSwitchIds:    vswitchIds,
			SystemDisk: &alicloudEmrClient.SystemDisk{
				Category: tea.String(plan.SystemDisk.Category.ValueString()),
				Size:     tea.Int32(int32(plan.SystemDisk.Size.ValueInt64())),
			},
		},
	}

	if !plan.NodeCount.IsNull() && !plan.NodeCount.IsUnknown() {
		createReq.NodeGroup.NodeCount = tea.Int32(int32(plan.NodeCount.ValueInt64()))
	}

	if !plan.SpotStrategy.IsNull() && !plan.SpotStrategy.IsUnknown() {
		createReq.NodeGroup.SpotStrategy = tea.String(plan.SpotStrategy.ValueString())
	}

	if !plan.WithPublicIp.IsNull() && !plan.WithPublicIp.IsUnknown() {
		createReq.NodeGroup.WithPublicIp = tea.Bool(plan.WithPublicIp.ValueBool())
	}

	if !plan.PaymentType.IsNull() && !plan.PaymentType.IsUnknown() {
		createReq.NodeGroup.PaymentType = tea.String(plan.PaymentType.ValueString())
	}

	if !plan.GracefulShutdown.IsNull() && !plan.GracefulShutdown.IsUnknown() {
		createReq.NodeGroup.GracefulShutdown = tea.Bool(plan.GracefulShutdown.ValueBool())
	}

	if !plan.SpotInstanceRemedy.IsNull() && !plan.SpotInstanceRemedy.IsUnknown() {
		createReq.NodeGroup.SpotInstanceRemedy = tea.Bool(plan.SpotInstanceRemedy.ValueBool())
	}

	if !plan.SystemDisk.Count.IsNull() && !plan.SystemDisk.Count.IsUnknown() {
		createReq.NodeGroup.SystemDisk.Count = tea.Int32(int32(plan.SystemDisk.Count.ValueInt64()))
	}

	if plan.SubscriptionConfig != nil {
		subscriptionConfig := &alicloudEmrClient.SubscriptionConfig{
			PaymentDuration:     tea.Int32(int32(plan.SubscriptionConfig.PaymentDuration.ValueInt64())),
			PaymentDurationUnit: tea.String(plan.SubscriptionConfig.PaymentDurationUnit.ValueString()),
		}
		if !plan.SubscriptionConfig.AutoRenew.IsNull() && !plan.SubscriptionConfig.AutoRenew.IsUnknown() {
			subscriptionConfig.AutoRenew = tea.Bool(plan.SubscriptionConfig.AutoRenew.ValueBool())
		}
		if !plan.SubscriptionConfig.AutoRenewDuration.IsNull() && !plan.SubscriptionConfig.AutoRenewDuration.IsUnknown() {
			subscriptionConfig.AutoRenewDuration = tea.Int32(int32(plan.SubscriptionConfig.AutoRenewDuration.ValueInt64()))
		}
		if !plan.SubscriptionConfig.AutoRenewDurationUnit.IsNull() && !plan.SubscriptionConfig.AutoRenewDurationUnit.IsUnknown() {
			subscriptionConfig.AutoRenewDurationUnit = tea.String(plan.SubscriptionConfig.AutoRenewDurationUnit.ValueString())
		}
		createReq.NodeGroup.SubscriptionConfig = subscriptionConfig
	}

	for _, dd := range plan.DataDisks {
		dataDisk := &alicloudEmrClient.DataDisk{
			Category: tea.String(dd.Category.ValueString()),
			Size:     tea.Int32(int32(dd.Size.ValueInt64())),
		}
		if !dd.Count.IsNull() && !dd.Count.IsUnknown() {
			dataDisk.Count = tea.Int32(int32(dd.Count.ValueInt64()))
		}
		createReq.NodeGroup.DataDisks = append(createReq.NodeGroup.DataDisks, dataDisk)
	}

	runtime := &util.RuntimeOptions{}
	createResp, err := r.client.CreateNodeGroupWithOptions(createReq, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create EMR node group", err.Error())
		return
	}

	nodeGroupId := tea.StringValue(createResp.Body.NodeGroupId)
	plan.NodeGroupId = types.StringValue(nodeGroupId)

	if err := r.waitForNodeGroupActive(plan.ClusterId.ValueString(), nodeGroupId, 30*time.Minute); err != nil {
		resp.Diagnostics.AddError("Timed out waiting for node group to become active", err.Error())
		return
	}

	resp.Diagnostics.Append(r.getNodeGroup(ctx, plan.ClusterId.ValueString(), nodeGroupId, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emrClusterNodeGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *emrClusterNodeGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.getNodeGroup(ctx, state.ClusterId.ValueString(), state.NodeGroupId.ValueString(), state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emrClusterNodeGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state *emrClusterNodeGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := &alicloudEmrClient.UpdateNodeGroupAttributesRequest{
		ClusterId:   tea.String(plan.ClusterId.ValueString()),
		NodeGroupId: tea.String(state.NodeGroupId.ValueString()),
	}

	if !plan.NodeGroupName.Equal(state.NodeGroupName) {
		updateReq.NodeGroupName = tea.String(plan.NodeGroupName.ValueString())
		state.NodeGroupName = plan.NodeGroupName
	}

	if !plan.InstanceTypes.Equal(state.InstanceTypes) {
		var instanceTypes []*string
		resp.Diagnostics.Append(plan.InstanceTypes.ElementsAs(ctx, &instanceTypes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateReq.InstanceTypeList = instanceTypes
		state.InstanceTypes = plan.InstanceTypes
	}

	if !plan.NodeCount.IsNull() && !plan.NodeCount.Equal(state.NodeCount) {
		updateReq.NodeCount = tea.Int32(int32(plan.NodeCount.ValueInt64()))
		state.NodeCount = plan.NodeCount
	}

	if !plan.SpotStrategy.IsNull() && !plan.SpotStrategy.Equal(state.SpotStrategy) {
		updateReq.EcsSpotStrategy = tea.String(plan.SpotStrategy.ValueString())
		state.SpotStrategy = plan.SpotStrategy
	}

	if !plan.GracefulShutdown.IsNull() && !plan.GracefulShutdown.Equal(state.GracefulShutdown) {
		updateReq.EnableGracefulDecommission = tea.Bool(plan.GracefulShutdown.ValueBool())
		state.GracefulShutdown = plan.GracefulShutdown
	}

	if !plan.SpotInstanceRemedy.IsNull() && !plan.SpotInstanceRemedy.Equal(state.SpotInstanceRemedy) {
		updateReq.SpotInstanceRemedy = tea.Bool(plan.SpotInstanceRemedy.ValueBool())
		state.SpotInstanceRemedy = plan.SpotInstanceRemedy
	}

	_, err := r.client.UpdateNodeGroupAttributesWithOptions(updateReq, &util.RuntimeOptions{})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update EMR node group", err.Error())
		return
	}

	if err := r.waitForNodeGroupActive(plan.ClusterId.ValueString(), state.NodeGroupId.ValueString(), 30*time.Minute); err != nil {
		resp.Diagnostics.AddError("Timed out waiting for node group to become active", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emrClusterNodeGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *emrClusterNodeGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.DeleteNodeGroupWithOptions(&alicloudEmrClient.DeleteNodeGroupRequest{
		ClusterId:   tea.String(state.ClusterId.ValueString()),
		NodeGroupId: tea.String(state.NodeGroupId.ValueString()),
	}, &util.RuntimeOptions{})
	if err != nil && !isNotFoundError(err) {
		resp.Diagnostics.AddError("Failed to delete EMR node group", err.Error())
		return
	}
}

func (r *emrClusterNodeGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected import ID format: <cluster_id>:<node_group_id>
	idParts := strings.Split(req.ID, ":")

	state := &emrClusterNodeGroupModel{}
	resp.Diagnostics.Append(r.getNodeGroup(ctx, idParts[0], idParts[1], state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ClusterId = types.StringValue(idParts[0])
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// isNotFoundError reports whether err represents an AliCloud EMR "not found"
// API error (e.g. the cluster or node group has already been removed).
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if sdkErr, ok := err.(*tea.SDKError); ok && sdkErr.Code != nil {
		code := *sdkErr.Code
		return strings.Contains(code, "NotFound") || strings.Contains(code, "EntityNotExist")
	}
	return strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "EntityNotExist")
}

func (r *emrClusterNodeGroupResource) waitForNodeGroupActive(clusterId, nodeGroupId string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := r.client.GetNodeGroup(&alicloudEmrClient.GetNodeGroupRequest{
			RegionId:    r.client.RegionId,
			ClusterId:   tea.String(clusterId),
			NodeGroupId: tea.String(nodeGroupId),
		})
		if err != nil {
			return err
		}
		state := tea.StringValue(resp.Body.NodeGroup.NodeGroupState)
		switch state {
		case "RUNNING", "ACTIVE":
			return nil
		case "FAILED":
			return fmt.Errorf("node group %s entered FAILED state", nodeGroupId)
		}
		time.Sleep(15 * time.Second)
	}
	return fmt.Errorf("timed out after %s waiting for node group %s to become active", timeout, nodeGroupId)
}

func (r *emrClusterNodeGroupResource) getNodeGroup(ctx context.Context, clusterId, nodeGroupId string, state *emrClusterNodeGroupModel) diag.Diagnostics {
	getResp, err := r.client.GetNodeGroup(&alicloudEmrClient.GetNodeGroupRequest{
		ClusterId:   tea.String(clusterId),
		NodeGroupId: tea.String(nodeGroupId),
	})
	if err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic("Failed to read EMR node group", err.Error())}
	}

	ng := getResp.Body.NodeGroup
	var diags diag.Diagnostics

	state.NodeGroupId = types.StringValue(tea.StringValue(ng.NodeGroupId))
	state.NodeGroupName = types.StringValue(tea.StringValue(ng.NodeGroupName))
	state.NodeGroupType = types.StringValue(tea.StringValue(ng.NodeGroupType))
	state.InstanceTypes, diags = types.ListValueFrom(ctx, types.StringType, ng.InstanceTypes)
	if diags.HasError() {
		return diags
	}
	state.VSwitchIds, diags = types.ListValueFrom(ctx, types.StringType, ng.VSwitchIds)
	if diags.HasError() {
		return diags
	}
	state.NodeCount = types.Int64Value(int64(tea.Int32Value(ng.RunningNodeCount)))
	state.SpotStrategy = types.StringValue(tea.StringValue(ng.SpotStrategy))
	state.WithPublicIp = types.BoolValue(tea.BoolValue(ng.WithPublicIp))
	state.PaymentType = types.StringValue(tea.StringValue(ng.PaymentType))
	state.GracefulShutdown = types.BoolValue(tea.BoolValue(ng.GracefulShutdown))
	state.SpotInstanceRemedy = types.BoolValue(tea.BoolValue(ng.SpotInstanceRemedy))
	if ng.SystemDisk != nil {
		state.SystemDisk = &diskModel{
			Category: types.StringValue(tea.StringValue(ng.SystemDisk.Category)),
			Size:     types.Int64Value(int64(tea.Int32Value(ng.SystemDisk.Size))),
			Count:    types.Int64Value(1),
		}
	}
	var dds []*diskModel
	for _, dd := range ng.DataDisks {
		if dd != nil {
			dds = append(dds, &diskModel{
				Category: types.StringValue(tea.StringValue(dd.Category)),
				Size:     types.Int64Value(int64(tea.Int32Value(dd.Size))),
				Count:    types.Int64Value(int64(tea.Int32Value(dd.Count))),
			})
		}
	}
	state.DataDisks = dds

	return nil
}
