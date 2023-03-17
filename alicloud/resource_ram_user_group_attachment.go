package alicloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudRamClient "github.com/alibabacloud-go/ram-20150501/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ resource.Resource              = &ramUserGroupAttachmentResource{}
	_ resource.ResourceWithConfigure = &ramUserGroupAttachmentResource{}
)

func NewRamUserGroupAttachmentResource() resource.Resource {
	return &ramUserGroupAttachmentResource{}
}

type ramUserGroupAttachmentResource struct {
	client *alicloudRamClient.Client
}

type ramUserGroupAttachmentResourceModel struct {
	GroupName types.String `tfsdk:"group_name"`
	UserName  types.String `tfsdk:"user_name"`
}

func (r *ramUserGroupAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ram_user_group_attachment"
}

func (r *ramUserGroupAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Alicloud RAM User Group Attachment resource.",
		Attributes: map[string]schema.Attribute{
			"group_name": schema.StringAttribute{
				Description: "The group name.",
				Required:    true,
			},
			"user_name": schema.StringAttribute{
				Description: "The username of the RAM group member.",
				Required:    true,
			},
		},
	}
}

func (r *ramUserGroupAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).ramClient
}

func (r *ramUserGroupAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *ramUserGroupAttachmentResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	addUserToGroupRequest := &alicloudRamClient.AddUserToGroupRequest{
		UserName:  tea.String(plan.UserName.ValueString()),
		GroupName: tea.String(plan.GroupName.ValueString()),
	}

	err := r.addUserToGroup(addUserToGroupRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Add User to Group",
			err.Error(),
		)
		return
	}

	state := &ramUserGroupAttachmentResourceModel{}
	state.GroupName = plan.GroupName
	state.UserName = plan.UserName

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ramUserGroupAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *ramUserGroupAttachmentResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readUserForGroup := func() error {
		runtime := &util.RuntimeOptions{}

		listUserForGroupRequest := &alicloudRamClient.ListUsersForGroupRequest{
			GroupName: tea.String(state.GroupName.ValueString()),
		}

		listUserForGroupResponse, err := r.client.ListUsersForGroupWithOptions(listUserForGroupRequest, runtime)
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

		for _, user := range listUserForGroupResponse.Body.Users.User {
			if *user.UserName == state.UserName.ValueString() {
				return nil
			}
		}
		state.UserName = types.StringValue("")
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(readUserForGroup, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Read Users for Group",
			err.Error(),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ramUserGroupAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *ramUserGroupAttachmentResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateUserGroupRequest := &alicloudRamClient.AddUserToGroupRequest{
		UserName:  tea.String(plan.UserName.ValueString()),
		GroupName: tea.String(plan.GroupName.ValueString()),
	}

	err := r.addUserToGroup(updateUserGroupRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Add User to Group",
			err.Error(),
		)
		return
	}

	state := ramUserGroupAttachmentResourceModel{}
	state.GroupName = plan.GroupName
	state.UserName = plan.UserName

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ramUserGroupAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *ramUserGroupAttachmentResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	removeUserFromGroupRequest := &alicloudRamClient.RemoveUserFromGroupRequest{
		UserName:  tea.String(state.UserName.ValueString()),
		GroupName: tea.String(state.GroupName.ValueString()),
	}

	runtime := &util.RuntimeOptions{}

	_, err := r.client.RemoveUserFromGroupWithOptions(removeUserFromGroupRequest, runtime)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Remove User from Group",
			err.Error(),
		)
	}
}

func (r *ramUserGroupAttachmentResource) addUserToGroup(req *alicloudRamClient.AddUserToGroupRequest) (err error) {
	addUserToGroup := func() error {
		runtime := &util.RuntimeOptions{}

		_, err := r.client.AddUserToGroupWithOptions(req, runtime)
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

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	return backoff.Retry(addUserToGroup, reconnectBackoff)
}
