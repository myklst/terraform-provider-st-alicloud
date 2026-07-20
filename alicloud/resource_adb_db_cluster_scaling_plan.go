package alicloud

import (
	"context"
	"fmt"
	"strings"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudAdbClientdw "github.com/alibabacloud-go/adb-20190315/v6/client"
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
	client *alicloudAdbClientdw.Client
}

type adbScalingPlanModel struct {
	Id                       types.String `tfsdk:"id"`
	DBClusterId              types.String `tfsdk:"db_cluster_id"`
	ElasticPlanName          types.String `tfsdk:"elastic_plan_name"`
	ElasticPlanTimeStart     types.String `tfsdk:"elastic_plan_time_start"`
	ElasticPlanTimeEnd       types.String `tfsdk:"elastic_plan_time_end"`
	ElasticPlanType          types.String `tfsdk:"elastic_plan_type"`
	ElasticPlanEnable        types.Bool   `tfsdk:"elastic_plan_enable"`
	ElasticPlanStartDay      types.String `tfsdk:"elastic_plan_start_day"`
	ElasticPlanEndDay        types.String `tfsdk:"elastic_plan_end_day"`
	ElasticPlanWorkerSpec    types.String `tfsdk:"elastic_plan_worker_spec"`
	ElasticPlanNodeNum       types.Int64  `tfsdk:"elastic_plan_node_num"`
	ElasticPlanWeeklyRepeat  types.String `tfsdk:"elastic_plan_weekly_repeat"`
	ElasticPlanMonthlyRepeat types.String `tfsdk:"elastic_plan_monthly_repeat"`
	ResourcePoolName         types.String `tfsdk:"resource_pool_name"`
}

func (r *adbScalingPlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "__adb_db_cluster_scaling_plan"
}

func (r *adbScalingPlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"db_cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"elastic_plan_name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"elastic_plan_time_start":     schema.StringAttribute{Required: true},
			"elastic_plan_time_end":       schema.StringAttribute{Required: true},
			"elastic_plan_type":           schema.StringAttribute{Required: true},
			"elastic_plan_enable":         schema.BoolAttribute{Required: true},
			"elastic_plan_start_day":      schema.StringAttribute{Optional: true},
			"elastic_plan_end_day":        schema.StringAttribute{Optional: true},
			"elastic_plan_worker_spec":    schema.StringAttribute{Optional: true},
			"elastic_plan_node_num":       schema.Int64Attribute{Optional: true},
			"elastic_plan_weekly_repeat":  schema.StringAttribute{Optional: true},
			"elastic_plan_monthly_repeat": schema.StringAttribute{Optional: true},
			"resource_pool_name":          schema.StringAttribute{Optional: true},
		},
	}
}

func (r *adbScalingPlanResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).adbClientdw
}

func (r *adbScalingPlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan adbScalingPlanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &alicloudAdbClientdw.CreateElasticPlanRequest{
		DBClusterId:          tea.String(plan.DBClusterId.ValueString()),
		ElasticPlanName:      tea.String(plan.ElasticPlanName.ValueString()),
		ElasticPlanTimeStart: tea.String(plan.ElasticPlanTimeStart.ValueString()),
		ElasticPlanTimeEnd:   tea.String(plan.ElasticPlanTimeEnd.ValueString()),
		ElasticPlanType:      tea.String(plan.ElasticPlanType.ValueString()),
		ElasticPlanEnable:    tea.Bool(plan.ElasticPlanEnable.ValueBool()),
	}

	if !plan.ElasticPlanStartDay.IsNull() && !plan.ElasticPlanStartDay.IsUnknown() && plan.ElasticPlanStartDay.ValueString() != "" {createReq.ElasticPlanStartDay = tea.String(plan.ElasticPlanStartDay.ValueString())}
	if !plan.ElasticPlanEndDay.IsNull() && !plan.ElasticPlanEndDay.IsUnknown() && plan.ElasticPlanEndDay.ValueString() != "" {createReq.ElasticPlanEndDay = tea.String(plan.ElasticPlanEndDay.ValueString())}
	if !plan.ElasticPlanWorkerSpec.IsNull() && !plan.ElasticPlanWorkerSpec.IsUnknown() && plan.ElasticPlanWorkerSpec.ValueString() != "" {createReq.ElasticPlanWorkerSpec = tea.String(plan.ElasticPlanWorkerSpec.ValueString())}
	if !plan.ElasticPlanNodeNum.IsNull() && !plan.ElasticPlanNodeNum.IsUnknown() {createReq.ElasticPlanNodeNum = tea.Int32(int32(plan.ElasticPlanNodeNum.ValueInt64()))}
	if !plan.ElasticPlanWeeklyRepeat.IsNull() && !plan.ElasticPlanWeeklyRepeat.IsUnknown() && plan.ElasticPlanWeeklyRepeat.ValueString() != "" {createReq.ElasticPlanWeeklyRepeat = tea.String(plan.ElasticPlanWeeklyRepeat.ValueString())}
	if !plan.ElasticPlanMonthlyRepeat.IsNull() && !plan.ElasticPlanMonthlyRepeat.IsUnknown() && plan.ElasticPlanMonthlyRepeat.ValueString() != "" {createReq.ElasticPlanMonthlyRepeat = tea.String(plan.ElasticPlanMonthlyRepeat.ValueString())}
	if !plan.ResourcePoolName.IsNull() && !plan.ResourcePoolName.IsUnknown() && plan.ResourcePoolName.ValueString() != "" {createReq.ResourcePoolName = tea.String(plan.ResourcePoolName.ValueString())}

	runtime := &util.RuntimeOptions{}
	_, err := r.client.CreateElasticPlanWithOptions(createReq, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Create ADB DW Scaling Plan", err.Error())
		return
	}

	plan.Id = types.StringValue(fmt.Sprintf("%s:%s", plan.DBClusterId.ValueString(), plan.ElasticPlanName.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *adbScalingPlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state adbScalingPlanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	describeReq := &alicloudAdbClientdw.DescribeElasticPlanRequest{
		DBClusterId:     tea.String(state.DBClusterId.ValueString()),
		ElasticPlanName: tea.String(state.ElasticPlanName.ValueString()),
	}

	runtime := &util.RuntimeOptions{}
	listResp, err := r.client.DescribeElasticPlanWithOptions(describeReq, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Read ADB DW Scaling Plans", err.Error())
		return
	}

	planFound := false
	if listResp != nil && listResp.Body != nil && listResp.Body.ElasticPlanList != nil {
		for _, p := range listResp.Body.ElasticPlanList {
			if p != nil && p.PlanName != nil && *p.PlanName == state.ElasticPlanName.ValueString() {
				planFound = true

				if p.Enable != nil { state.ElasticPlanEnable = types.BoolValue(*p.Enable) }
				if p.ElasticNodeNum != nil { state.ElasticPlanNodeNum = types.Int64Value(int64(*p.ElasticNodeNum)) }
				if p.ElasticPlanType != nil { state.ElasticPlanType = types.StringValue(*p.ElasticPlanType) }
				if p.ElasticPlanWorkerSpec != nil { state.ElasticPlanWorkerSpec = types.StringValue(*p.ElasticPlanWorkerSpec) }
				if p.EndDay != nil { state.ElasticPlanEndDay = types.StringValue(*p.EndDay) }
				if p.EndTime != nil { state.ElasticPlanTimeEnd = types.StringValue(*p.EndTime) }
				if p.MonthlyRepeat != nil { state.ElasticPlanMonthlyRepeat = types.StringValue(*p.MonthlyRepeat) }
				if p.ResourcePoolName != nil { state.ResourcePoolName = types.StringValue(*p.ResourcePoolName) }
				if p.StartDay != nil { state.ElasticPlanStartDay = types.StringValue(*p.StartDay) }
				if p.StartTime != nil { state.ElasticPlanTimeStart = types.StringValue(*p.StartTime) }
				if p.WeeklyRepeat != nil { state.ElasticPlanWeeklyRepeat = types.StringValue(*p.WeeklyRepeat) }

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
	var plan adbScalingPlanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	modifyReq := &alicloudAdbClientdw.ModifyElasticPlanRequest{
		DBClusterId:          tea.String(plan.DBClusterId.ValueString()),
		ElasticPlanName:      tea.String(plan.ElasticPlanName.ValueString()),
		ElasticPlanTimeStart: tea.String(plan.ElasticPlanTimeStart.ValueString()),
		ElasticPlanTimeEnd:   tea.String(plan.ElasticPlanTimeEnd.ValueString()),
		ElasticPlanType:      tea.String(plan.ElasticPlanType.ValueString()),
		ElasticPlanEnable:    tea.Bool(plan.ElasticPlanEnable.ValueBool()),
	}

	if !plan.ElasticPlanStartDay.IsNull() && !plan.ElasticPlanStartDay.IsUnknown() && plan.ElasticPlanStartDay.ValueString() != "" {modifyReq.ElasticPlanStartDay = tea.String(plan.ElasticPlanStartDay.ValueString())}
	if !plan.ElasticPlanEndDay.IsNull() && !plan.ElasticPlanEndDay.IsUnknown() && plan.ElasticPlanEndDay.ValueString() != "" {modifyReq.ElasticPlanEndDay = tea.String(plan.ElasticPlanEndDay.ValueString())}
	if !plan.ElasticPlanWorkerSpec.IsNull() && !plan.ElasticPlanWorkerSpec.IsUnknown() && plan.ElasticPlanWorkerSpec.ValueString() != "" {modifyReq.ElasticPlanWorkerSpec = tea.String(plan.ElasticPlanWorkerSpec.ValueString())}
	if !plan.ElasticPlanNodeNum.IsNull() && !plan.ElasticPlanNodeNum.IsUnknown() {modifyReq.ElasticPlanNodeNum = tea.Int32(int32(plan.ElasticPlanNodeNum.ValueInt64()))}
	if !plan.ElasticPlanWeeklyRepeat.IsNull() && !plan.ElasticPlanWeeklyRepeat.IsUnknown() && plan.ElasticPlanWeeklyRepeat.ValueString() != "" {modifyReq.ElasticPlanWeeklyRepeat = tea.String(plan.ElasticPlanWeeklyRepeat.ValueString())}
	if !plan.ElasticPlanMonthlyRepeat.IsNull() && !plan.ElasticPlanMonthlyRepeat.IsUnknown() && plan.ElasticPlanMonthlyRepeat.ValueString() != "" {modifyReq.ElasticPlanMonthlyRepeat = tea.String(plan.ElasticPlanMonthlyRepeat.ValueString())}
	if !plan.ResourcePoolName.IsNull() && !plan.ResourcePoolName.IsUnknown() && plan.ResourcePoolName.ValueString() != "" {modifyReq.ResourcePoolName = tea.String(plan.ResourcePoolName.ValueString())}

	runtime := &util.RuntimeOptions{}
	_, err := r.client.ModifyElasticPlanWithOptions(modifyReq, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Modify ADB DW Scaling Plan", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *adbScalingPlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state adbScalingPlanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteReq := &alicloudAdbClientdw.DeleteElasticPlanRequest{
		DBClusterId:     tea.String(state.DBClusterId.ValueString()),
		ElasticPlanName: tea.String(state.ElasticPlanName.ValueString()),
	}

	runtime := &util.RuntimeOptions{}
	_, err := r.client.DeleteElasticPlanWithOptions(deleteReq, runtime)
	if err != nil {
		if !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "InvalidCluster") {
			resp.Diagnostics.AddError("Failed to Delete ADB DW Scaling Plan", err.Error())
		}
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
