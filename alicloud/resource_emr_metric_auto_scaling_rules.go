package alicloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudEmrClient "github.com/alibabacloud-go/emr-20210320/client"
)

var (
	_ resource.Resource              = &emrMetricAutoScalingRulesResource{}
	_ resource.ResourceWithConfigure = &emrMetricAutoScalingRulesResource{}
)

func NewEmrMetricAutoScalingRulesResource() resource.Resource {
	return &emrMetricAutoScalingRulesResource{}
}

type emrMetricAutoScalingRulesResource struct {
	client *alicloudEmrClient.Client
}

type emrMetricAutoScalingRulesModel struct {
	ClusterId    types.String   `tfsdk:"cluster_id"`
	MaximumNodes types.Int64    `tfsdk:"max_nodes"`
	MinimumNodes types.Int64    `tfsdk:"min_nodes"`
	NodeGroupId  types.String   `tfsdk:"node_group_id"`
	ScalingRule  []*scalingRule `tfsdk:"scaling_rule"`
}

type scalingRule struct {
	RuleName                types.String  `tfsdk:"rule_name"`
	MutliMetricRelationship types.String  `tfsdk:"multi_metric_relationship"`
	StaticticalPeriod       types.Int64   `tfsdk:"statistical_period"`
	EvaluationCount         types.Int64   `tfsdk:"evaluation_count"`
	ScaleOperation          types.String  `tfsdk:"scale_operation"`
	ScalingNodeCount        types.Int64   `tfsdk:"scaling_node_count"`
	CooldownTime            types.Int64   `tfsdk:"cooldown_time"`
	MetricRule              []*metricRule `tfsdk:"metric_rule"`
}

type metricRule struct {
	MetricName         types.String  `tfsdk:"metric_name"`
	ComparisonOperator types.String  `tfsdk:"comparison_operator"`
	StatisticalMeasure types.String  `tfsdk:"statistical_measure"`
	Threshold          types.Float64 `tfsdk:"threshold"`
}

// Metadata returns the SSL binding resource name.
func (r *emrMetricAutoScalingRulesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_emr_metric_auto_scaling_rules"
}

// Schema defines the schema for the SSL certificate binding resource.
func (r *emrMetricAutoScalingRulesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Auto scaling rule for AliCloud E-MapReduce cluster nodes.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Description: "Alicloud E-MapReduce cluster ID.",
				Required:    true,
			},
			"node_group_id": schema.StringAttribute{
				Description: "Alicloud E-MapReduce cluster task node group ID.",
				Computed:    true,
			},
			"max_nodes": schema.Int64Attribute{
				Description: "Maximum capacity of scaling for nodes.",
				Required:    true,
			},
			"min_nodes": schema.Int64Attribute{
				Description: "Minimum capacity of scaling for nodes.",
				Required:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"scaling_rule": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"rule_name": schema.StringAttribute{
							Description: "Auto scaling rule name.",
							Required:    true,
						},
						"multi_metric_relationship": schema.StringAttribute{
							Description: "Determines whether auto scaling triggers when all metrics trigger (And) or any metrics are triggered (Or). Accepted Values: \"And\", \"Or\".",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("And", "Or"),
							},
						},
						"statistical_period": schema.Int64Attribute{
							Description: "Period to collect cluster load metrics.",
							Required:    true,
						},
						"evaluation_count": schema.Int64Attribute{
							Description: "Amount of times scaling is triggered. (Repetitions that trigger scale out.)",
							Required:    true,
						},
						"scale_operation": schema.StringAttribute{
							Description: "Scaling mode, scale out (up) or scale in (down). Accepted values: \"SCALE_OUT\", \"SCALE_IN\"",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("SCALE_OUT", "SCALE_IN"),
							},
						},
						"scaling_node_count": schema.Int64Attribute{
							Description: "Number of nodes being scaled up or down.",
							Required:    true,
						},
						"cooldown_time": schema.Int64Attribute{
							Description: "The interval between two scaling activities. During the cooldown time, auto scaling is not performed even if the required rule is triggered.",
							Optional:    true,
						},
					},
					Blocks: map[string]schema.Block{
						"metric_rule": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"metric_name": schema.StringAttribute{
										Description: "Metric name.",
										Required:    true,
									},
									"comparison_operator": schema.StringAttribute{
										Description: "Comparison operator. Accepted values: \"EQ\": equals, \"NE\": not equals, \"GT\": greater than, \"LT\": lesser than, \"GE\": greater or equal, \"LE\": lesser or equal.",
										Required:    true,
										Validators: []validator.String{
											stringvalidator.OneOf("EQ", "NE", "GT", "LT", "GE", "LE"),
										},
									},
									"statistical_measure": schema.StringAttribute{
										Description: "Statistical measure. Accepted values: \"AVG\", \"MIN\", \"MAX\".",
										Required:    true,
										Validators: []validator.String{
											stringvalidator.OneOf("AVG", "MIN", "MAX"),
										},
									},
									"threshold": schema.Float64Attribute{
										Description: "Threshold percentage of metric to trigger auto scaling.",
										Required:    true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *emrMetricAutoScalingRulesResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).emrClient
}

// Create a new SSL cert and domain binding
func (r *emrMetricAutoScalingRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *emrMetricAutoScalingRulesModel
	var state *emrMetricAutoScalingRulesModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	nodeGroupId, err := r.getNodeGroup(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to get node group",
			err.Error(),
		)
		return
	}

	err = r.putRule(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to create auto scaling rule",
			err.Error(),
		)
		return
	}

	state = plan
	state.NodeGroupId = types.StringValue(nodeGroupId)

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read web rules configuration for SSL cert and domain binding
func (r *emrMetricAutoScalingRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *emrMetricAutoScalingRulesModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var autoScalingPolicy *alicloudEmrClient.GetAutoScalingPolicyResponse
	var err error

	readAutoScalingRules := func() error {
		runtime := &util.RuntimeOptions{}

		getAutoScalingPolicyRequest := &alicloudEmrClient.GetAutoScalingPolicyRequest{
			RegionId:    r.client.RegionId,
			NodeGroupId: tea.String(state.NodeGroupId.ValueString()),
			ClusterId:   tea.String(state.ClusterId.ValueString()),
		}

		autoScalingPolicy, err = r.client.GetAutoScalingPolicyWithOptions(getAutoScalingPolicyRequest, runtime)
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
	err = backoff.Retry(readAutoScalingRules, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to read auto scaling rules.",
			err.Error(),
		)
		return
	}

	var metricRules []*metricRule
	var scalingRules []*scalingRule
	for _, scale := range autoScalingPolicy.Body.ScalingPolicy.ScalingRules {
		for _, rule := range scale.MetricsTrigger.Conditions {
			metricRules = append(metricRules, &metricRule{
				MetricName:         types.StringValue(*rule.MetricName),
				ComparisonOperator: types.StringValue(*rule.ComparisonOperator),
				StatisticalMeasure: types.StringValue(*rule.Statistics),
				Threshold:          types.Float64Value(*rule.Threshold),
			})
		}

		scalingRules = append(scalingRules, &scalingRule{
			RuleName:                types.StringValue(*scale.RuleName),
			MutliMetricRelationship: types.StringValue(*scale.MetricsTrigger.ConditionLogicOperator),
			StaticticalPeriod:       types.Int64Value(int64(*scale.MetricsTrigger.TimeWindow)),
			EvaluationCount:         types.Int64Value(int64(*scale.MetricsTrigger.EvaluationCount)),
			ScaleOperation:          types.StringValue(*scale.ActivityType),
			ScalingNodeCount:        types.Int64Value(int64(*scale.AdjustmentValue)),
			CooldownTime:            types.Int64Value(int64(*scale.MetricsTrigger.CoolDownInterval)),
			MetricRule:              metricRules,
		})
	}

	state = &emrMetricAutoScalingRulesModel{
		ClusterId:    types.StringValue(*autoScalingPolicy.Body.ScalingPolicy.ClusterId),
		MaximumNodes: types.Int64Value(int64(*autoScalingPolicy.Body.ScalingPolicy.Constraints.MaxCapacity)),
		MinimumNodes: types.Int64Value(int64(*autoScalingPolicy.Body.ScalingPolicy.Constraints.MinCapacity)),
		NodeGroupId:  types.StringValue(*autoScalingPolicy.Body.ScalingPolicy.NodeGroupId),
		ScalingRule:  scalingRules,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update binds new SSL cert to domain and sets the updated Terraform state on success.
func (r *emrMetricAutoScalingRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *emrMetricAutoScalingRulesModel
	var state *emrMetricAutoScalingRulesModel

	// Retrieve values from plan
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	nodeGroupId, err := r.getNodeGroup(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to get node group",
			err.Error(),
		)
		return
	}

	err = r.putRule(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to update auto scaling rules.",
			err.Error(),
		)
		return
	}

	state = plan
	state.NodeGroupId = types.StringValue(nodeGroupId)

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// SSL cert could not be unbinded, will always remain.
func (r *emrMetricAutoScalingRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *emrMetricAutoScalingRulesModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteAutoScalingRules := func() error {
		runtime := &util.RuntimeOptions{}

		removeAutoScalingPolicyRequest := &alicloudEmrClient.RemoveAutoScalingPolicyRequest{
			RegionId:    r.client.RegionId,
			ClusterId:   tea.String(state.ClusterId.ValueString()),
			NodeGroupId: tea.String(state.NodeGroupId.ValueString()),
		}

		_, err := r.client.RemoveAutoScalingPolicyWithOptions(removeAutoScalingPolicyRequest, runtime)
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
	err := backoff.Retry(deleteAutoScalingRules, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to delete auto scaling rules.",
			err.Error(),
		)
		return
	}
}

func (r *emrMetricAutoScalingRulesResource) getNodeGroup(plan *emrMetricAutoScalingRulesModel) (string, error) {
	var nodeGroup *alicloudEmrClient.ListNodeGroupsResponse
	var err error

	listNodeGroup := func() error {
		runtime := &util.RuntimeOptions{}

		listNodeGroupsRequest := &alicloudEmrClient.ListNodeGroupsRequest{
			RegionId:       r.client.RegionId,
			ClusterId:      tea.String(plan.ClusterId.ValueString()),
			NodeGroupTypes: []*string{tea.String("TASK")},
		}

		nodeGroup, err = r.client.ListNodeGroupsWithOptions(listNodeGroupsRequest, runtime)
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
	err = backoff.Retry(listNodeGroup, reconnectBackoff)
	if err != nil {
		return "", err
	}

	return *nodeGroup.Body.NodeGroups[0].NodeGroupId, nil
}

// Function to bind certificate to domain
func (r *emrMetricAutoScalingRulesResource) putRule(plan *emrMetricAutoScalingRulesModel) error {

	putRule := func() error {
		runtime := &util.RuntimeOptions{}
		var scalingRules []*alicloudEmrClient.ScalingRule

		scalingConstraints := &alicloudEmrClient.ScalingConstraints{
			MaxCapacity: tea.Int32(int32(plan.MaximumNodes.ValueInt64())),
			MinCapacity: tea.Int32(int32(plan.MinimumNodes.ValueInt64())),
		}

		for _, scale := range plan.ScalingRule {

			var triggerConditions []*alicloudEmrClient.TriggerCondition
			for _, rule := range scale.MetricRule {
				triggerConditions = append(triggerConditions,
					&alicloudEmrClient.TriggerCondition{
						Threshold:          tea.Float64(rule.Threshold.ValueFloat64()),
						ComparisonOperator: tea.String(rule.ComparisonOperator.ValueString()),
						Statistics:         tea.String(rule.StatisticalMeasure.ValueString()),
						MetricName:         tea.String(rule.MetricName.ValueString()),
					},
				)
			}

			scalingRuleMetricsTrigger := &alicloudEmrClient.MetricsTrigger{
				TimeWindow:             tea.Int32(int32(scale.StaticticalPeriod.ValueInt64())),
				EvaluationCount:        tea.Int32(int32(scale.EvaluationCount.ValueInt64())),
				CoolDownInterval:       tea.Int32(int32(scale.CooldownTime.ValueInt64())),
				ConditionLogicOperator: tea.String(scale.MutliMetricRelationship.ValueString()),
				Conditions:             triggerConditions,
			}

			scalingRules = append(scalingRules,
				&alicloudEmrClient.ScalingRule{
					RuleName:        tea.String(scale.RuleName.ValueString()),
					TriggerType:     tea.String("METRICS_TRIGGER"),
					ActivityType:    tea.String(scale.ScaleOperation.ValueString()),
					AdjustmentValue: tea.Int32(int32(scale.ScalingNodeCount.ValueInt64())),
					MetricsTrigger:  scalingRuleMetricsTrigger,
				},
			)
		}

		nodeGroupId, err := r.getNodeGroup(plan)
		if err != nil {
			return err
		}

		putAutoScalingPolicyRequest := &alicloudEmrClient.PutAutoScalingPolicyRequest{
			RegionId:     r.client.RegionId,
			ClusterId:    tea.String(plan.ClusterId.ValueString()),
			NodeGroupId:  &nodeGroupId,
			Constraints:  scalingConstraints,
			ScalingRules: scalingRules,
		}

		_, err = r.client.PutAutoScalingPolicyWithOptions(putAutoScalingPolicyRequest, runtime)
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
	err := backoff.Retry(putRule, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}
