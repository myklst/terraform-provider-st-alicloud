package alicloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudAdbClient "github.com/alibabacloud-go/adb-20190315/v2/client"
)

var (
	_ resource.Resource              = &aliadbResourceGroupBindResource{}
	_ resource.ResourceWithConfigure = &aliadbResourceGroupBindResource{}
)

func NewAliadbResourceGroupBindResource() resource.Resource {
	return &aliadbResourceGroupBindResource{}
}

type aliadbResourceGroupBindResource struct {
	client *alicloudAdbClient.Client
}

type aliadbResourceGroupBindResourceModel struct {
	// Required
	DBClusterId types.String `tfsdk:"dbcluster_id"`
	GroupName   types.String `tfsdk:"group_name"`
	GroupUser   types.String `tfsdk:"group_user"`
}

// Metadata returns the resource alicloud adb resource group association type name.
func (r *aliadbResourceGroupBindResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aliadb_resource_group_bind_user"
}

func (r *aliadbResourceGroupBindResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Aliadb resource group association resource.",
		Attributes: map[string]schema.Attribute{
			"dbcluster_id": schema.StringAttribute{
				Description: "The ID of the AnalyticDB for MySQL Data Warehouse Edition (V3.0) cluster.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_name": schema.StringAttribute{
				Description: "The name of the resource group.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_user": schema.StringAttribute{
				Description: "The database account with which to associate the resource group.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *aliadbResourceGroupBindResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).adbClient
}

// Create a new DNS weight resource
func (r *aliadbResourceGroupBindResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *aliadbResourceGroupBindResourceModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Bind user to resource group
	err := r.bindGroupUser(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Bind Group User.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &aliadbResourceGroupBindResourceModel{}
	state.DBClusterId = plan.DBClusterId
	state.GroupName = plan.GroupName
	state.GroupUser = plan.GroupUser

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read resource group user bind resource information
func (r *aliadbResourceGroupBindResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Alicloud doesn't provide a SDK for describing resource group user binding, the read function will not be implemented.
}

// Update updates the DNS weight resource and sets the updated Terraform state on success.
func (r *aliadbResourceGroupBindResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *aliadbResourceGroupBindResourceModel

	// Retrieve values from plan
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.unbindGroupUser(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to unbind resource group with user.",
			err.Error(),
		)
		return
	}

	if err := r.bindGroupUser(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to bind resource group with user.",
			err.Error(),
		)
		return
	}

	// Set state values
	state := &aliadbResourceGroupBindResourceModel{}
	state.DBClusterId = plan.DBClusterId
	state.GroupName = plan.GroupName
	state.GroupUser = plan.GroupUser

	// Set state to plan data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete the resource group user bind resource and removes the Terraform state on success.
func (r *aliadbResourceGroupBindResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *aliadbResourceGroupBindResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.unbindGroupUser(state); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to unbind resource group with user.",
			err.Error(),
		)
		return
	}
}

func (r *aliadbResourceGroupBindResource) bindGroupUser(plan *aliadbResourceGroupBindResourceModel) error {
	bindGroupUser := func() error {
		runtime := &util.RuntimeOptions{}

		// Look for SubDomain Name
		bindDBResourceGroupWithUserRequest := &alicloudAdbClient.BindDBResourceGroupWithUserRequest{
			DBClusterId: tea.String(plan.DBClusterId.ValueString()),
			GroupName:   tea.String(plan.GroupName.ValueString()),
			GroupUser:   tea.String(plan.GroupUser.ValueString()),
		}

		_, err := r.client.BindDBResourceGroupWithUserWithOptions(bindDBResourceGroupWithUserRequest, runtime)
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
	err := backoff.Retry(bindGroupUser, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}

func (r *aliadbResourceGroupBindResource) unbindGroupUser(plan *aliadbResourceGroupBindResourceModel) error {
	setRecordWeight := func() error {
		runtime := &util.RuntimeOptions{}

		unbindDBResourceGroupWithUserRequest := &alicloudAdbClient.UnbindDBResourceGroupWithUserRequest{
			DBClusterId: tea.String(plan.DBClusterId.ValueString()),
			GroupName:   tea.String(plan.GroupName.ValueString()),
			GroupUser:   tea.String(plan.GroupUser.ValueString()),
		}

		_, err := r.client.UnbindDBResourceGroupWithUserWithOptions(unbindDBResourceGroupWithUserRequest, runtime)
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
	err := backoff.Retry(setRecordWeight, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}
