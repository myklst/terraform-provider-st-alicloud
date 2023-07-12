package alicloud

import (
	"context"
	"encoding/json"
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
	_ resource.Resource               = &cmsAlarmRuleResource{}
	_ resource.ResourceWithConfigure  = &cmsAlarmRuleResource{}
	_ resource.ResourceWithModifyPlan = &cmsAlarmRuleResource{}
)

func NewCmsAlarmRuleResource() resource.Resource {
	return &cmsAlarmRuleResource{}
}

type cmsAlarmRuleResource struct {
	client *alicloudCmsClient.Client
}

type cmsAlarmRuleResourceModel struct {
	RuleId              types.String      `tfsdk:"rule_id"`
	RuleName            types.String      `tfsdk:"rule_name"`
	Namespace           types.String      `tfsdk:"namespace"`
	MetricName          types.String      `tfsdk:"metric_name"`
	Resources           []*resourceConfig `tfsdk:"resources"`
	ContactGroups       types.String      `tfsdk:"contact_groups"`
	CompositeExpression expressionConfig  `tfsdk:"composite_expression"`
}

type expressionConfig struct {
	ExpressionRaw types.String `tfsdk:"expression_raw"`
	Level         types.String `tfsdk:"level"`
	Times         types.Int64  `tfsdk:"times"`
}

type resourceConfig struct {
	ResourceCategory types.String `tfsdk:"resource_category"`
	ResourceValue    types.String `tfsdk:"resource_value"`
}

// Metadata returns the resource CMS Alarm Rule type name.
func (r *cmsAlarmRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cms_alarm_rule"
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
			"resources": schema.ListNestedAttribute{
				Description: "List of alarm rule resource configurations.",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"resource_category": schema.StringAttribute{
							Description: "Alarm rule resource category.",
							Required:    true,
						},
						"resource_value": schema.StringAttribute{
							Description: "Alarm rule resource value.",
							Required:    true,
						},
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
			"[API ERROR] Failed to Set Alarm Rule",
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
	state.Resources = plan.Resources
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

		if alarmRuleResponse.String() != "{}" {
			state.RuleName = types.StringValue(*alarmRuleResponse.Body.Alarms.Alarm[0].RuleName)
			state.Namespace = types.StringValue(*alarmRuleResponse.Body.Alarms.Alarm[0].Namespace)
			state.MetricName = types.StringValue(*alarmRuleResponse.Body.Alarms.Alarm[0].MetricName)
			state.ContactGroups = types.StringValue(*alarmRuleResponse.Body.Alarms.Alarm[0].ContactGroups)
			state.CompositeExpression.ExpressionRaw = types.StringValue(*alarmRuleResponse.Body.Alarms.Alarm[0].CompositeExpression.ExpressionRaw)
			state.CompositeExpression.Level = types.StringValue(*alarmRuleResponse.Body.Alarms.Alarm[0].CompositeExpression.Level)
			state.CompositeExpression.Times = types.Int64Value(int64(*alarmRuleResponse.Body.Alarms.Alarm[0].CompositeExpression.Times))

			jsonString := *alarmRuleResponse.Body.Alarms.Alarm[0].Resources
			var processedString []map[string]string

			_err := json.Unmarshal([]byte(jsonString), &processedString)
			if _err != nil {
				return _err
			}

			for _, resource := range processedString {
				config := &resourceConfig{}
				for key, value := range resource {
					config.ResourceCategory = types.StringValue(key)
					config.ResourceValue = types.StringValue(value)
				}
				state.Resources = append(state.Resources, config)
			}
		} else {
			state.RuleId = types.StringNull()
			state.RuleName = types.StringNull()
			state.Namespace = types.StringNull()
			state.MetricName = types.StringNull()
			state.Resources = []*resourceConfig{}
			state.ContactGroups = types.StringNull()
			state.CompositeExpression.ExpressionRaw = types.StringNull()
			state.CompositeExpression.Level = types.StringNull()
			state.CompositeExpression.Times = types.Int64Null()
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second

	err := backoff.Retry(readAlarmRule, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Read CMS Alarm Rule",
			err.Error(),
		)
		return
	}

	// Set refreshed state
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
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
			"[API ERROR] Failed to Set Alarm Rule",
			err.Error(),
		)
		return
	}

	// Set state items
	state.RuleName = plan.RuleName
	state.Namespace = plan.Namespace
	state.MetricName = plan.MetricName
	state.Resources = plan.Resources
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
			"[API ERROR] Failed to Delete CMS Alarm Rule",
			err.Error(),
		)
		return
	}
}

func (r *cmsAlarmRuleResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If the entire plan is null, the resource is planned for destruction.
	if !(req.Plan.Raw.IsNull()) {
		var plan *cmsAlarmRuleResourceModel
		getPlanDiags := req.Plan.Get(ctx, &plan)
		resp.Diagnostics.Append(getPlanDiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		resp.Plan.Set(ctx, &plan)
		if resp.Diagnostics.HasError() {
			return
		}
	}
}

func (r *cmsAlarmRuleResource) setRule(ctx context.Context, plan *cmsAlarmRuleResourceModel, ruleId string) error {
	setAlarmRule := func() error {
		runtime := &util.RuntimeOptions{}

		var resources []map[string]string

		for _, x := range plan.Resources {
			resource := map[string]string{
				x.ResourceCategory.ValueString(): x.ResourceValue.ValueString(),
			}
			resources = append(resources, resource)
		}

		jsonData, _err := json.Marshal(resources)
		if _err != nil {
			return _err
		}
		jsonString := string(jsonData)

		compositeExpression := &alicloudCmsClient.PutResourceMetricRuleRequestCompositeExpression{
			ExpressionRaw: tea.String(plan.CompositeExpression.ExpressionRaw.ValueString()),
			Level:         tea.String(plan.CompositeExpression.Level.ValueString()),
			Times:         tea.Int32(int32(plan.CompositeExpression.Times.ValueInt64())),
		}

		putResourceMetricRuleRequest := &alicloudCmsClient.PutResourceMetricRuleRequest{
			RuleId:              tea.String(ruleId),
			RuleName:            tea.String(plan.RuleName.ValueString()),
			Namespace:           tea.String(plan.Namespace.ValueString()),
			MetricName:          tea.String(plan.MetricName.ValueString()),
			Resources:           tea.String(jsonString),
			ContactGroups:       tea.String(plan.ContactGroups.ValueString()),
			CompositeExpression: compositeExpression,
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
