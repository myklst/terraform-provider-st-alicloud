package alicloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudAdbClientV4 "github.com/alibabacloud-go/adb-20211201/v4/client"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ resource.Resource                = &adbScalingPlanResource{}
	_ resource.ResourceWithConfigure   = &adbScalingPlanResource{}
	_ resource.ResourceWithImportState = &adbScalingPlanResource{}
)

func NewAliadbScalingPlanResource() resource.Resource {
	return &adbScalingPlanResource{}
}

type adbScalingPlanResource struct {
	client *alicloudAdbClientV4.Client
}

type adbScalingPlanResourceModel struct {
	Id                types.String `tfsdk:"id"`
	DBClusterId       types.String `tfsdk:"db_cluster_id"`
	ElasticPlanName   types.String `tfsdk:"elastic_plan_name"`
	Type              types.String `tfsdk:"type"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	TargetSize        types.String `tfsdk:"target_size"`
	ResourceGroupName types.String `tfsdk:"resource_group_name"`
	CronExpression    types.String `tfsdk:"cron_expression"`
	StartTime         types.String `tfsdk:"start_time"`
	EndTime           types.String `tfsdk:"end_time"`
	AutoScale         types.Bool   `tfsdk:"auto_scale"`
}

func (r *adbScalingPlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aliadb_scaling_plan"
}

func (r *adbScalingPlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"db_cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"elastic_plan_name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				Required: true,
			},
			"target_size": schema.StringAttribute{
				Optional: true,
			},
			"resource_group_name": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cron_expression": schema.StringAttribute{
				Optional: true,
			},
			"start_time": schema.StringAttribute{
				Optional: true,
			},
			"end_time": schema.StringAttribute{
				Optional: true,
			},
			"auto_scale": schema.BoolAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *adbScalingPlanResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).adbClientV4
}

func (r *adbScalingPlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan adbScalingPlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &alicloudAdbClientV4.CreateElasticPlanRequest{
		DBClusterId:     tea.String(plan.DBClusterId.ValueString()),
		ElasticPlanName: tea.String(plan.ElasticPlanName.ValueString()),
		Type:            tea.String(plan.Type.ValueString()),
		Enabled:         tea.Bool(plan.Enabled.ValueBool()), // Set Enable/Disable on creation
	}

	if !plan.TargetSize.IsNull() {
		createReq.TargetSize = tea.String(plan.TargetSize.ValueString())
	}
	if !plan.ResourceGroupName.IsNull() {
		createReq.ResourceGroupName = tea.String(plan.ResourceGroupName.ValueString())
	}
	if !plan.CronExpression.IsNull() {
		createReq.CronExpression = tea.String(plan.CronExpression.ValueString())
	}
	if !plan.StartTime.IsNull() {
		createReq.StartTime = tea.String(plan.StartTime.ValueString())
	}
	if !plan.EndTime.IsNull() {
		createReq.EndTime = tea.String(plan.EndTime.ValueString())
	}
	if !plan.AutoScale.IsNull() {
		createReq.AutoScale = tea.Bool(plan.AutoScale.ValueBool())
	}

	runtime := &util.RuntimeOptions{}
	_, err := r.client.CreateElasticPlanWithOptions(createReq, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Create ADB Scaling Plan", err.Error())
		return
	}

	plan.Id = types.StringValue(fmt.Sprintf("%s:%s", plan.DBClusterId.ValueString(), plan.ElasticPlanName.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *adbScalingPlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state adbScalingPlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	describeReq := &alicloudAdbClientV4.DescribeElasticPlansRequest{
		DBClusterId:     tea.String(state.DBClusterId.ValueString()),
		ElasticPlanName: tea.String(state.ElasticPlanName.ValueString()),
		PageNumber:      tea.Int32(1),
		PageSize:        tea.Int32(50),
	}

	runtime := &util.RuntimeOptions{}
	listResp, err := r.client.DescribeElasticPlansWithOptions(describeReq, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Read ADB Scaling Plans", err.Error())
		return
	}

	planFound := false
	if listResp != nil && listResp.Body != nil && listResp.Body.ElasticPlans != nil {
		for _, p := range listResp.Body.ElasticPlans {
			if p != nil && p.ElasticPlanName != nil && *p.ElasticPlanName == state.ElasticPlanName.ValueString() {
				planFound = true

				if p.Type != nil {
					state.Type = types.StringValue(*p.Type)
				}
				if p.Enabled != nil {
					state.Enabled = types.BoolValue(*p.Enabled)
				}
				if p.TargetSize != nil {
					state.TargetSize = types.StringValue(*p.TargetSize)
				}

				if p.ResourceGroupName != nil {
					state.ResourceGroupName = types.StringValue(*p.ResourceGroupName)
				}

				break
			}
		}
	}

	if !planFound {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *adbScalingPlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state adbScalingPlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	runtime := &util.RuntimeOptions{}

	needsModification := (plan.TargetSize != state.TargetSize) ||
		(plan.CronExpression != state.CronExpression) ||
		(plan.StartTime != state.StartTime) ||
		(plan.EndTime != state.EndTime)

	if needsModification {
		modifyReq := &alicloudAdbClientV4.ModifyElasticPlanRequest{
			DBClusterId:     tea.String(plan.DBClusterId.ValueString()),
			ElasticPlanName: tea.String(plan.ElasticPlanName.ValueString()),
		}
		if !plan.TargetSize.IsNull() && !plan.TargetSize.IsUnknown() {
			modifyReq.TargetSize = tea.String(plan.TargetSize.ValueString())
		}
		if !plan.CronExpression.IsNull() && !plan.CronExpression.IsUnknown() {
			modifyReq.CronExpression = tea.String(plan.CronExpression.ValueString())
		}
		if !plan.StartTime.IsNull() && !plan.StartTime.IsUnknown() {
			modifyReq.StartTime = tea.String(plan.StartTime.ValueString())
		}
		if !plan.EndTime.IsNull() && !plan.EndTime.IsUnknown() {
			modifyReq.EndTime = tea.String(plan.EndTime.ValueString())
		}

		_, err := r.client.ModifyElasticPlanWithOptions(modifyReq, runtime)
		if err != nil {
			resp.Diagnostics.AddError("Failed to Modify ADB Scaling Plan", err.Error())
			return
		}
	}

	if plan.Enabled.ValueBool() != state.Enabled.ValueBool() {
		if plan.Enabled.ValueBool() {
			enableReq := &alicloudAdbClientV4.EnableElasticPlanRequest{
				DBClusterId:     tea.String(plan.DBClusterId.ValueString()),
				ElasticPlanName: tea.String(plan.ElasticPlanName.ValueString()),
			}
			_, err := r.client.EnableElasticPlanWithOptions(enableReq, runtime)
			if err != nil {
				resp.Diagnostics.AddError("Failed to Enable ADB Scaling Plan", err.Error())
				return
			}
		} else {
			disableReq := &alicloudAdbClientV4.DisableElasticPlanRequest{
				DBClusterId:     tea.String(plan.DBClusterId.ValueString()),
				ElasticPlanName: tea.String(plan.ElasticPlanName.ValueString()),
			}
			_, err := r.client.DisableElasticPlanWithOptions(disableReq, runtime)
			if err != nil {
				resp.Diagnostics.AddError("Failed to Disable ADB Scaling Plan", err.Error())
				return
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *adbScalingPlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state adbScalingPlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	runtime := &util.RuntimeOptions{}

	if state.Enabled.ValueBool() {
		disableReq := &alicloudAdbClientV4.DisableElasticPlanRequest{
			DBClusterId:     tea.String(state.DBClusterId.ValueString()),
			ElasticPlanName: tea.String(state.ElasticPlanName.ValueString()),
		}
		_, err := r.client.DisableElasticPlanWithOptions(disableReq, runtime)
		if err != nil {
			if !strings.Contains(err.Error(), "already disabled") && !strings.Contains(err.Error(), "NotFound") {
				resp.Diagnostics.AddWarning("Failed to disable plan prior to destruction", err.Error())
			}
		}
		time.Sleep(2 * time.Second)
	}

	deleteReq := &alicloudAdbClientV4.DeleteElasticPlanRequest{
		DBClusterId:     tea.String(state.DBClusterId.ValueString()),
		ElasticPlanName: tea.String(state.ElasticPlanName.ValueString()),
	}

	_, err := r.client.DeleteElasticPlanWithOptions(deleteReq, runtime)
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "InvalidCluster") {
			return
		}
		resp.Diagnostics.AddError("Failed to Delete ADB Scaling Plan", err.Error())
		return
	}
}

func (r *adbScalingPlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, ":")

	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import format: 'db_cluster_id:elastic_plan_name'. Got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("db_cluster_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("elastic_plan_name"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
