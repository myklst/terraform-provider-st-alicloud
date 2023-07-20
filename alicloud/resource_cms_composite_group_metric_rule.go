package alicloud

import (
	"context"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudCmsClient "github.com/alibabacloud-go/cms-20190101/v8/client"
)

var (
	_ resource.Resource              = &cmsAlarmRuleResource{}
	_ resource.ResourceWithConfigure = &cmsAlarmRuleResource{}
)

func NewCmsAlarmRuleResource() resource.Resource {
	return &cmsAlarmRuleResource{}
}

type cmsAlarmRuleResource struct {
	client *alicloudCmsClient.Client
}

type cmsAlarmRuleResourceModel struct {
	RuleId              types.String     `tfsdk:"rule_id"`
	RuleName            types.String     `tfsdk:"rule_name"`
	GroupId             types.Int64      `tfsdk:"group_id"`
	Namespace           types.String     `tfsdk:"namespace"`
	MetricName          types.String     `tfsdk:"metric_name"`
	ContactGroups       types.String     `tfsdk:"contact_groups"`
	CompositeExpression expressionConfig `tfsdk:"composite_expression"`
}

type expressionConfig struct {
	ExpressionRaw types.String `tfsdk:"expression_raw"`
	Level         types.String `tfsdk:"level"`
	Times         types.Int64  `tfsdk:"times"`
}

// Metadata returns the resource CMS Alarm Rule type name.
func (r *cmsAlarmRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cms_composite_group_metric_rule"
}

// Schema defines the schema for the CMS Alarm Rule resource.
func (r *cmsAlarmRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Cloud Monitor Service alarm rule resource.",
		Attributes: map[string]schema.Attribute{
			"rule_id": schema.StringAttribute{
				Description: "Alarm Rule Id.",
				Computed:    true,
			},
			"rule_name": schema.StringAttribute{
				Description: "Alarm Rule Name.",
				Required:    true,
			},
			"group_id": schema.Int64Attribute{
				Description: "Monitoring Group Rule Id.",
				Required:    true,
			},
			"namespace": schema.StringAttribute{
				Description: "Alarm Namespace.",
				Required:    true,
			},
			"metric_name": schema.StringAttribute{
				Description: "Alarm Metric Name.",
				Required:    true,
			},
			"contact_groups": schema.StringAttribute{
				Description: "Alarm Contact Groups.",
				Required:    true,
			},
			"composite_expression": schema.SingleNestedAttribute{
				Description: "The composite expression configuration for alarms.",
				Required:    true,
				Attributes: map[string]schema.Attribute{
					"expression_raw": schema.StringAttribute{
						Description: "Alarm rule expression.",
						Required:    true,
					},
					"level": schema.StringAttribute{
						Description: "Alarm alert level.",
						Required:    true,
					},
					"times": schema.Int64Attribute{
						Description: "Alarm retry times.",
						Required:    true,
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *cmsAlarmRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).cmsClient
}

// Create a new CMS Alarm Rule resource
func (r *cmsAlarmRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *cmsAlarmRuleResourceModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ruleUUID := uuid.New().String()

	// Set CMS Alarm Rule
	err := r.setRule(ctx, plan, ruleUUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Set Group Metric Rule",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &cmsAlarmRuleResourceModel{}
	state.RuleId = types.StringValue(ruleUUID)
	state.RuleName = plan.RuleName
	state.Namespace = plan.Namespace
	state.MetricName = plan.MetricName
	state.GroupId = plan.GroupId
	state.ContactGroups = plan.ContactGroups
	state.CompositeExpression = plan.CompositeExpression

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read CMS Alarm Rule resource information
func (r *cmsAlarmRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *cmsAlarmRuleResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Retry backoff function
	readAlarmRule := func() error {
		runtime := &util.RuntimeOptions{}

		// Read CMS Alarm Rule Values on Console
		describeMetricRuleListRequest := &alicloudCmsClient.DescribeMetricRuleListRequest{
			RuleIds: tea.String(state.RuleId.ValueString()),
		}

		alarmRuleResponse, err := r.client.DescribeMetricRuleListWithOptions(describeMetricRuleListRequest, runtime)
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

		totalRules, _ := strconv.ParseInt(*alarmRuleResponse.Body.Total, 10, 64)

		if totalRules > 0 &&
			alarmRuleResponse.Body.Alarms.Alarm[0].CompositeExpression.ExpressionRaw != nil &&
			alarmRuleResponse.Body.Alarms.Alarm[0].CompositeExpression.Level != nil &&
			alarmRuleResponse.Body.Alarms.Alarm[0].CompositeExpression.Times != nil {

			alarm := alarmRuleResponse.Body.Alarms.Alarm[0]
			groupId, _ := strconv.ParseInt(*alarmRuleResponse.Body.Alarms.Alarm[0].GroupId, 10, 64)

			state.RuleName = types.StringValue(*alarm.RuleName)
			state.Namespace = types.StringValue(*alarm.Namespace)
			state.MetricName = types.StringValue(*alarm.MetricName)
			state.ContactGroups = types.StringValue(*alarm.ContactGroups)
			state.GroupId = types.Int64Value(groupId)

			state.CompositeExpression.ExpressionRaw = types.StringValue(*alarm.CompositeExpression.ExpressionRaw)
			state.CompositeExpression.Level = types.StringValue(*alarm.CompositeExpression.Level)
			state.CompositeExpression.Times = types.Int64Value(int64(*alarm.CompositeExpression.Times))

			// Set refreshed state
			setStateDiags := resp.State.Set(ctx, &state)
			resp.Diagnostics.Append(setStateDiags...)
			if resp.Diagnostics.HasError() {
				resp.Diagnostics.AddError(
					"[API ERROR] Failed to Set Read CMS Group Metric Rule to State",
					err.Error(),
				)
			}
		} else {
			resp.State.RemoveResource(ctx)
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second

	err := backoff.Retry(readAlarmRule, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Read CMS Group Metric Rule",
			err.Error(),
		)
		return
	}
}

// Update updates the CMS Alarm Rule resource and sets the updated Terraform state on success.
func (r *cmsAlarmRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *cmsAlarmRuleResourceModel
	var state *cmsAlarmRuleResourceModel

	// Retrieve values from plan
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set CMS Alarm Rule
	err := r.setRule(ctx, plan, state.RuleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Update CMS Group Metric Rule",
			err.Error(),
		)
		return
	}

	// Set state items
	state.RuleName = plan.RuleName
	state.Namespace = plan.Namespace
	state.MetricName = plan.MetricName
	state.GroupId = plan.GroupId
	state.ContactGroups = plan.ContactGroups
	state.CompositeExpression = plan.CompositeExpression

	// Set state to plan data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the CMS alarm rule resource and removes the Terraform state on success.
func (r *cmsAlarmRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *cmsAlarmRuleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteAlarmRule := func() error {
		runtime := &util.RuntimeOptions{}

		// Delete Alarm Rule
		deleteMetricRulesRequest := &alicloudCmsClient.DeleteMetricRulesRequest{
			Id: []*string{tea.String(state.RuleId.ValueString())},
		}

		_, err := r.client.DeleteMetricRulesWithOptions(deleteMetricRulesRequest, runtime)
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

	err := backoff.Retry(deleteAlarmRule, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Delete CMS Group Metric Rule",
			err.Error(),
		)
		return
	}
}

func (r *cmsAlarmRuleResource) setRule(ctx context.Context, plan *cmsAlarmRuleResourceModel, ruleId string) error {
	setAlarmRule := func() error {
		runtime := &util.RuntimeOptions{}

		// Placeholder Rule to be replaced by PutResourceMetricRule,
		// Alicloud API requires these values when creating a group metric rule.
		createGroupMetricRulesRequest := &alicloudCmsClient.CreateGroupMetricRulesRequest{
			GroupId: tea.Int64(plan.GroupId.ValueInt64()),
			GroupMetricRules: []*alicloudCmsClient.CreateGroupMetricRulesRequestGroupMetricRules{
				{
					MetricName: tea.String("CPUUtilization"),
					RuleId:     tea.String(ruleId),
					Namespace:  tea.String("acs_ecs_dashboard"),
					RuleName:   tea.String(plan.RuleName.ValueString()),
					Escalations: &alicloudCmsClient.CreateGroupMetricRulesRequestGroupMetricRulesEscalations{
						Critical: &alicloudCmsClient.CreateGroupMetricRulesRequestGroupMetricRulesEscalationsCritical{
							Times:              tea.Int32(5),
							Threshold:          tea.String("75"),
							Statistics:         tea.String("Average"),
							ComparisonOperator: tea.String("GreaterThanOrEqualToThreshold"),
						},
					},
				},
			},
		}

		_, _err := r.client.CreateGroupMetricRulesWithOptions(createGroupMetricRulesRequest, runtime)
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

		putResourceMetricRuleRequest := &alicloudCmsClient.PutResourceMetricRuleRequest{
			RuleId:        tea.String(ruleId),
			RuleName:      tea.String(plan.RuleName.ValueString()),
			Namespace:     tea.String(plan.Namespace.ValueString()),
			MetricName:    tea.String(plan.MetricName.ValueString()),
			Resources:     tea.String("[{\"\":\"\"}]"), // Resources will be replaced by Monitoring Group Resources
			ContactGroups: tea.String(plan.ContactGroups.ValueString()),
			CompositeExpression: &alicloudCmsClient.PutResourceMetricRuleRequestCompositeExpression{
				ExpressionRaw: tea.String(plan.CompositeExpression.ExpressionRaw.ValueString()),
				Level:         tea.String(plan.CompositeExpression.Level.ValueString()),
				Times:         tea.Int32(int32(plan.CompositeExpression.Times.ValueInt64())),
			},
		}

		_, err := r.client.PutResourceMetricRuleWithOptions(putResourceMetricRuleRequest, runtime)
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
	err := backoff.Retry(setAlarmRule, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}
