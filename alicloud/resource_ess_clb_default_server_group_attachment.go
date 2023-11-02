package alicloud

import (
	"context"
	"fmt"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudEssClient "github.com/alibabacloud-go/ess-20220222/v2/client"
)

var (
	_ resource.Resource              = &essClbDefaultServerGroupAttachmentResource{}
	_ resource.ResourceWithConfigure = &essClbDefaultServerGroupAttachmentResource{}
)

func NewEssClbDefaultServerGroupAttachmentResource() resource.Resource {
	return &essClbDefaultServerGroupAttachmentResource{}
}

type essClbDefaultServerGroupAttachmentResource struct {
	client *alicloudEssClient.Client
}

type essClbDefaultServerGroupAttachmentModel struct {
	ScalingGroupId  types.String `tfsdk:"scaling_group_id"`
	LoadBalancerIds types.List   `tfsdk:"load_balancer_ids"`
}

// Metadata returns the ESS CLB Default Server Group Attachment resource name.
func (r *essClbDefaultServerGroupAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ess_clb_default_server_group_attachment"
}

// Schema defines the schema for the ESS CLB Default Server Group Attachment resource.
func (r *essClbDefaultServerGroupAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attach an auto scaling group (ESS) with a list of load balancers (CLB) default server group.",
		Attributes: map[string]schema.Attribute{
			"scaling_group_id": schema.StringAttribute{
				Description: "Scaling Group ID.",
				Required:    true,
			},
			"load_balancer_ids": schema.ListAttribute{
				Description: "List of load balancer IDs.",
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *essClbDefaultServerGroupAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).essClient
}

// Attach scaling group with load balancers' default server group.
func (r *essClbDefaultServerGroupAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *essClbDefaultServerGroupAttachmentModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.attachLoadBalancers(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to attach scaling group with load balancers' default server group.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &essClbDefaultServerGroupAttachmentModel{
		ScalingGroupId:  plan.ScalingGroupId,
		LoadBalancerIds: plan.LoadBalancerIds,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read the attached load balancers in the scaling group.
func (r *essClbDefaultServerGroupAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *essClbDefaultServerGroupAttachmentModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	loadBalancerIds, scalingGroupId, err := r.getLoadBalancersFromScalingGroup(state)
	if err != nil {
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to get attached load balancers from scaling group.",
				err.Error(),
			)
			return
		}
	}

	state = &essClbDefaultServerGroupAttachmentModel{
		ScalingGroupId:  types.StringValue(scalingGroupId),
		LoadBalancerIds: types.ListValueMust(types.StringType, loadBalancerIds),
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update the attachment of scaling group with load balancers' default server group.
func (r *essClbDefaultServerGroupAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan *essClbDefaultServerGroupAttachmentModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state *essClbDefaultServerGroupAttachmentModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	loadBalancerIds, scalingGroupId, err := r.getLoadBalancersFromScalingGroup(state)
	if err != nil {
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to get load balancers from scaling group.",
				err.Error(),
			)
			return
		}
	}

	if plan.ScalingGroupId == types.StringValue(scalingGroupId) {
		stateLbs := make(map[string]struct{})
		planLbs := make(map[string]struct{})

		for _, lb := range loadBalancerIds {
			stateLbs[trimStringQuotes(lb.String())] = struct{}{}
		}
		for _, lb := range plan.LoadBalancerIds.Elements() {
			planLbs[trimStringQuotes(lb.String())] = struct{}{}
		}

		// Detach load balancer when load balancer from State does not exist in Plan.
		var detachLbs []attr.Value
		for _, lb := range loadBalancerIds {
			if _, exists := planLbs[trimStringQuotes(lb.String())]; !exists {
				detachLbs = append(detachLbs, types.StringValue(trimStringQuotes(lb.String())))
			}
		}
		if len(detachLbs) > 0 {
			state.LoadBalancerIds = types.ListValueMust(types.StringType, detachLbs)
			err = r.detachLoadBalancers(state)
			if err != nil {
				resp.Diagnostics.AddError(
					"[API ERROR] Failed to detach load balancers with scaling group.",
					err.Error(),
				)
				return
			}
		}

		// Attach load balancer when load balancer from Plan does not exist in State.
		var attachLbs []attr.Value
		for _, lb := range plan.LoadBalancerIds.Elements() {
			if _, exists := stateLbs[trimStringQuotes(lb.String())]; !exists {
				attachLbs = append(attachLbs, types.StringValue(trimStringQuotes(lb.String())))
			}
		}
		if len(attachLbs) > 0 {
			state.LoadBalancerIds = types.ListValueMust(types.StringType, attachLbs)
			err = r.attachLoadBalancers(plan)
			if err != nil {
				resp.Diagnostics.AddError(
					"[API ERROR] Failed to attach scaling group with load balancers' default server group.",
					err.Error(),
				)
				return
			}
		}
	} else {
		// attach a new scaling group with load balancers' default server group
		err = r.attachLoadBalancers(plan)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to attach scaling group with load balancers' default server group.",
				err.Error(),
			)
			return
		}

		// detach an old scaling group with load balancers' default server group
		err = r.detachLoadBalancers(state)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to detach scaling group with load balancers' default server group.",
				err.Error(),
			)
			return
		}
	}

	// Set state items
	state = &essClbDefaultServerGroupAttachmentModel{
		ScalingGroupId:  plan.ScalingGroupId,
		LoadBalancerIds: plan.LoadBalancerIds,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Detach scaling group with load balancers' default server group.
func (r *essClbDefaultServerGroupAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *essClbDefaultServerGroupAttachmentModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.detachLoadBalancers(state)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to detach scaling group with load balancers' default server group.",
			err.Error(),
		)
		return
	}
}

// Function to read the attached load balancers in a scaling group.
func (r *essClbDefaultServerGroupAttachmentResource) getLoadBalancersFromScalingGroup(model *essClbDefaultServerGroupAttachmentModel) ([]attr.Value, string, error) {
	var describeScalingGroupsResponse *alicloudEssClient.DescribeScalingGroupsResponse
	var err error
	var loadBalancers []attr.Value
	var scalingGroupId string

	// Retry backoff function
	describeScalingGroups := func() error {
		runtime := &util.RuntimeOptions{}

		describeScalingGroupsRequest := &alicloudEssClient.DescribeScalingGroupsRequest{
			RegionId: r.client.RegionId,
			ScalingGroupIds: []*string{tea.String(model.ScalingGroupId.ValueString())},
		}

		describeScalingGroupsResponse, err = r.client.DescribeScalingGroupsWithOptions(describeScalingGroupsRequest, runtime)
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
	err = backoff.Retry(describeScalingGroups, reconnectBackoff)
	if err != nil {
		return loadBalancers, scalingGroupId, err
	}

	for _, scalingGroup := range describeScalingGroupsResponse.Body.ScalingGroups {
		for _, loadBalancer := range scalingGroup.LoadBalancerIds {
			loadBalancers = append(loadBalancers, types.StringValue(*loadBalancer))
		}
		scalingGroupId = *scalingGroup.ScalingGroupId
	}
	return loadBalancers, scalingGroupId, nil
}

// Function to attach scaling group with load balancers' default server group.
func (r *essClbDefaultServerGroupAttachmentResource) attachLoadBalancers(model *essClbDefaultServerGroupAttachmentModel) error {
	attachLoadBalancers := func() error {
		runtime := &util.RuntimeOptions{}
		var loadBalancersIds []*string

		for _, id := range model.LoadBalancerIds.Elements() {
			fmt.Print(id)
			loadBalancersIds = append(loadBalancersIds, tea.String(trimStringQuotes(id.String())))
		}

		attachLoadBalancersRequest := &alicloudEssClient.AttachLoadBalancersRequest{
			ScalingGroupId: tea.String(model.ScalingGroupId.ValueString()),
			LoadBalancers:  loadBalancersIds,
			ForceAttach:    tea.Bool(true),
		}

		_, _err := r.client.AttachLoadBalancersWithOptions(attachLoadBalancersRequest, runtime)
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
	err := backoff.Retry(attachLoadBalancers, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}

// Function to detach scaling group with load balancers' default server group.
func (r *essClbDefaultServerGroupAttachmentResource) detachLoadBalancers(model *essClbDefaultServerGroupAttachmentModel) error {
	detachLoadBalancers := func() error {
		runtime := &util.RuntimeOptions{}
		var loadBalancersIds []*string

		for _, id := range model.LoadBalancerIds.Elements() {
			loadBalancersIds = append(loadBalancersIds, tea.String(trimStringQuotes(id.String())))
		}

		detachLoadBalancersRequest := &alicloudEssClient.DetachLoadBalancersRequest{
			ScalingGroupId: tea.String(model.ScalingGroupId.ValueString()),
			LoadBalancers:  loadBalancersIds,
			ForceDetach:    tea.Bool(true),
		}

		_, _err := r.client.DetachLoadBalancersWithOptions(detachLoadBalancersRequest, runtime)
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
	err := backoff.Retry(detachLoadBalancers, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}
