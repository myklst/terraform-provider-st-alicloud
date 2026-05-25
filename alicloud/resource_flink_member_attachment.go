package alicloud

import (
	"context"
	"fmt"
	"strings"

	util "github.com/alibabacloud-go/tea-utils/v2/service"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudVvpClient "github.com/alibabacloud-go/ververica-20220718/client"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ resource.Resource              = &ververicaMemberResource{}
	_ resource.ResourceWithConfigure = &ververicaMemberResource{}
)

func NewVervericaMemberResource() resource.Resource {
	return &ververicaMemberResource{}
}

type ververicaMemberResource struct {
	client *alicloudVvpClient.Client
}

type ververicaMemberResourceModel struct {
	Id          types.String `tfsdk:"id"`
	WorkspaceId types.String `tfsdk:"workspace_id"`
	Namespace   types.String `tfsdk:"namespace"`
	MemberId    types.String `tfsdk:"member_id"`
	Role        types.String `tfsdk:"role"`
}

func (r *ververicaMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_flink_member_attachment"
}

func (r *ververicaMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"workspace_id": schema.StringAttribute{
				Description: "The ID of the Flink Workspace.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "The namespace of the Ververica workspace.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"member_id": schema.StringAttribute{
				Description: "The RAM User ID to add as a member.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description: "The role of the member.",
				Required:    true,
			},
		},
	}
}

func (r *ververicaMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).ververicaClient
}

func (r *ververicaMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ververicaMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &alicloudVvpClient.CreateMemberRequest{
		Body: &alicloudVvpClient.Member{
			Member: tea.String(plan.MemberId.ValueString()),
			Role:   tea.String(plan.Role.ValueString()),
		},
	}

	headers := &alicloudVvpClient.CreateMemberHeaders{
		Workspace: tea.String(plan.WorkspaceId.ValueString()),
	}

	runtime := &util.RuntimeOptions{}

	_, err := r.client.CreateMemberWithOptions(tea.String(plan.Namespace.ValueString()), createReq, headers, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Create Ververica Member", err.Error())
		return
	}

	plan.Id = types.StringValue(fmt.Sprintf("%s:%s:%s", plan.WorkspaceId.ValueString(), plan.Namespace.ValueString(), plan.MemberId.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ververicaMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ververicaMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	listReq := &alicloudVvpClient.ListMembersRequest{}

	headers := &alicloudVvpClient.ListMembersHeaders{
		Workspace: tea.String(state.WorkspaceId.ValueString()),
	}

	runtime := &util.RuntimeOptions{}
	listResp, err := r.client.ListMembersWithOptions(tea.String(state.Namespace.ValueString()), listReq, headers, runtime)
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "InvalidWorkspace") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to Read Ververica Members", err.Error())
		return
	}

	memberFound := false

	if listResp != nil && listResp.Body != nil && listResp.Body.Data != nil {
		for _, member := range listResp.Body.Data {
			if member != nil && member.Member != nil && *member.Member == state.MemberId.ValueString() {
				memberFound = true
				if member.Role != nil {
					state.Role = types.StringValue(*member.Role)
				}
				break
			}
		}
	}

	if !memberFound {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ververicaMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ververicaMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Role.ValueString() != state.Role.ValueString() {
		updateReq := &alicloudVvpClient.UpdateMemberRequest{
			Body: &alicloudVvpClient.Member{
				Member: tea.String(plan.MemberId.ValueString()),
				Role:   tea.String(plan.Role.ValueString()),
			},
		}

		headers := &alicloudVvpClient.UpdateMemberHeaders{
			Workspace: tea.String(plan.WorkspaceId.ValueString()),
		}

		runtime := &util.RuntimeOptions{}

		_, err := r.client.UpdateMemberWithOptions(tea.String(plan.Namespace.ValueString()), updateReq, headers, runtime)
		if err != nil {
			resp.Diagnostics.AddError("Failed to Update Ververica Member", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ververicaMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ververicaMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	headers := &alicloudVvpClient.DeleteMemberHeaders{
		Workspace: tea.String(state.WorkspaceId.ValueString()),
	}

	runtime := &util.RuntimeOptions{}

	_, err := r.client.DeleteMemberWithOptions(
		tea.String(state.Namespace.ValueString()),
		tea.String(state.MemberId.ValueString()),
		headers,
		runtime,
	)

	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "InvalidWorkspace") {
			return
		}
		resp.Diagnostics.AddError("Failed to Delete Ververica Member", err.Error())
		return
	}
}
