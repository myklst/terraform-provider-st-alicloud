package alicloud

import (
	"context"
	"fmt"
	"strings"

	util "github.com/alibabacloud-go/tea-utils/v2/service"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	foasconsoleClient "github.com/alibabacloud-go/foasconsole-20211028/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ resource.Resource              = &foasconsoleNamespaceSpecResource{}
	_ resource.ResourceWithConfigure = &foasconsoleNamespaceSpecResource{}
)

func NewFoasconsoleNamespaceSpecResource() resource.Resource {
	return &foasconsoleNamespaceSpecResource{}
}

type foasconsoleNamespaceSpecResource struct {
	client *foasconsoleClient.Client
}

type resourceSpecModel struct {
	Cpu      types.Int64 `tfsdk:"cpu"`
	MemoryGB types.Int64 `tfsdk:"memory_gb"`
}

type foasconsoleNamespaceSpecResourceModel struct {
	Id                     types.String       `tfsdk:"id"`
	InstanceId             types.String       `tfsdk:"instance_id"`
	Namespace              types.String       `tfsdk:"namespace"`
	Ha                     types.Bool         `tfsdk:"ha"`
	GuaranteedResourceSpec *resourceSpecModel `tfsdk:"guaranteed_resource_spec"`
	ElasticResourceSpec    *resourceSpecModel `tfsdk:"elastic_resource_spec"`
}

func (r *foasconsoleNamespaceSpecResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_flink_namespace"
}

func (r *foasconsoleNamespaceSpecResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the Compute Unit (CU) Specs for an existing Alicloud Flink (foasconsole) Namespace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"instance_id": schema.StringAttribute{
				Description: "The Instance ID (Workspace ID) of the Flink project.",
				Required:    true,
			},
			"namespace": schema.StringAttribute{
				Description: "The name of the namespace.",
				Required:    true,
			},
			"ha": schema.BoolAttribute{
				Description: "High availability (HA) flag.",
				Required:    true,
			},
			"guaranteed_resource_spec": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"cpu": schema.Int64Attribute{
						Required: true,
					},
					"memory_gb": schema.Int64Attribute{
						Required: true,
					},
				},
			},
			"elastic_resource_spec": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"cpu": schema.Int64Attribute{
						Required: true,
					},
					"memory_gb": schema.Int64Attribute{
						Required: true,
					},
				},
			},
		},
	}
}

func (r *foasconsoleNamespaceSpecResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).foasconsoleClient
}

func (r *foasconsoleNamespaceSpecResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan foasconsoleNamespaceSpecResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build the API Request directly here
	modifyReq := &foasconsoleClient.ModifyNamespaceSpecV2Request{
		InstanceId: tea.String(plan.InstanceId.ValueString()),
		Namespace:  tea.String(plan.Namespace.ValueString()),
		Region:     r.client.RegionId,
		Ha:         tea.Bool(plan.Ha.ValueBool()),
	}

	if plan.GuaranteedResourceSpec != nil {
		modifyReq.GuaranteedResourceSpec = &foasconsoleClient.ModifyNamespaceSpecV2RequestGuaranteedResourceSpec{
			Cpu:      tea.Int32(int32(plan.GuaranteedResourceSpec.Cpu.ValueInt64())),
			MemoryGB: tea.Int32(int32(plan.GuaranteedResourceSpec.MemoryGB.ValueInt64())),
		}
	}

	if plan.ElasticResourceSpec != nil {
		modifyReq.ElasticResourceSpec = &foasconsoleClient.ModifyNamespaceSpecV2RequestElasticResourceSpec{
			Cpu:      tea.Int32(int32(plan.ElasticResourceSpec.Cpu.ValueInt64())),
			MemoryGB: tea.Int32(int32(plan.ElasticResourceSpec.MemoryGB.ValueInt64())),
		}
	}

	runtime := &util.RuntimeOptions{}

	// Pass it to the SDK Client
	_, err := r.client.ModifyNamespaceSpecV2WithOptions(modifyReq, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Create Namespace Spec", err.Error())
		return
	}

	plan.Id = types.StringValue(fmt.Sprintf("%s:%s", plan.InstanceId.ValueString(), plan.Namespace.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *foasconsoleNamespaceSpecResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state foasconsoleNamespaceSpecResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *foasconsoleNamespaceSpecResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan foasconsoleNamespaceSpecResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	modifyReq := &foasconsoleClient.ModifyNamespaceSpecV2Request{
		InstanceId: tea.String(plan.InstanceId.ValueString()),
		Namespace:  tea.String(plan.Namespace.ValueString()),
		Region:     r.client.RegionId,
		Ha:         tea.Bool(plan.Ha.ValueBool()),
	}

	if plan.GuaranteedResourceSpec != nil {
		modifyReq.GuaranteedResourceSpec = &foasconsoleClient.ModifyNamespaceSpecV2RequestGuaranteedResourceSpec{
			Cpu:      tea.Int32(int32(plan.GuaranteedResourceSpec.Cpu.ValueInt64())),
			MemoryGB: tea.Int32(int32(plan.GuaranteedResourceSpec.MemoryGB.ValueInt64())),
		}
	}

	if plan.ElasticResourceSpec != nil {
		modifyReq.ElasticResourceSpec = &foasconsoleClient.ModifyNamespaceSpecV2RequestElasticResourceSpec{
			Cpu:      tea.Int32(int32(plan.ElasticResourceSpec.Cpu.ValueInt64())),
			MemoryGB: tea.Int32(int32(plan.ElasticResourceSpec.MemoryGB.ValueInt64())),
		}
	}

	runtime := &util.RuntimeOptions{}

	_, err := r.client.ModifyNamespaceSpecV2WithOptions(modifyReq, runtime)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Update Namespace Spec", err.Error())
		return
	}

	plan.Id = types.StringValue(fmt.Sprintf("%s:%s", plan.InstanceId.ValueString(), plan.Namespace.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *foasconsoleNamespaceSpecResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Delete logic is empty because just modifying an existing workspace namespace.
}

func (r *foasconsoleNamespaceSpecResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, ":")

	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: instance_id:namespace. Got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), idParts[1])...)
}
