package alicloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ecdSimpleOfficeSiteResource{}
	_ resource.ResourceWithConfigure   = &ecdSimpleOfficeSiteResource{}
	_ resource.ResourceWithImportState = &ecdSimpleOfficeSiteResource{}
)

func NewAliecdSimpleOfficeSiteResource() resource.Resource {
	return &ecdSimpleOfficeSiteResource{}
}

type ecdSimpleOfficeSiteResource struct {
	client *EcdClient
}

type ECDBasicOfficeSiteModel struct {
	Id                       types.String `tfsdk:"id"`
	Bandwidth                types.Int64  `tfsdk:"bandwidth"`
	CenId                    types.String `tfsdk:"cen_id"`
	CenOwnerId               types.String `tfsdk:"cen_owner_id"`
	CidrBlock                types.String `tfsdk:"cidr_block"`
	DesktopAccessType        types.String `tfsdk:"desktop_access_type"`
	EnableAdminAccess        types.Bool   `tfsdk:"enable_admin_access"`
	EnableCrossDesktopAccess types.Bool   `tfsdk:"enable_cross_desktop_access"`
	EnableInternetAccess     types.Bool   `tfsdk:"enable_internet_access"`
	MfaEnabled               types.Bool   `tfsdk:"mfa_enabled"`
	OfficeSiteName           types.String `tfsdk:"office_site_name"`
	SsoEnabled               types.Bool   `tfsdk:"sso_enabled"`
	Status                   types.String `tfsdk:"status"`
	VpcType                  types.String `tfsdk:"vpc_type"`
	VpcId                    types.String `tfsdk:"vpc_id"`
}

// Metadata returns the ECD Simple Office Site resource type name.
func (r *ecdSimpleOfficeSiteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ecd_simple_office_site"
}

func (r *ecdSimpleOfficeSiteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Alicloud ECD Custom Simple Office Site Resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the Simple Office Site.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bandwidth": schema.Int64Attribute{
				Validators:         []validator.Int64{int64validator.Between(0, 200)},
				DeprecationMessage: "Field 'bandwidth' has been deprecated from provider version 1.142.0.",
				Optional:           true,
				Computed:           true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"cen_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cen_owner_id": schema.StringAttribute{
				Optional: true,
			},
			"cidr_block": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"desktop_access_type": schema.StringAttribute{
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.OneOf("Any", "Internet", "VPC")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_admin_access": schema.BoolAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"enable_cross_desktop_access": schema.BoolAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_internet_access": schema.BoolAttribute{
				Computed:           true,
				Optional:           true,
				DeprecationMessage: "Field 'enable_internet_access' has been deprecated from provider version 1.142.0.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"mfa_enabled": schema.BoolAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"office_site_name": schema.StringAttribute{
				Optional: true,
			},
			"sso_enabled": schema.BoolAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_type": schema.StringAttribute{
				Computed:   true,
				Optional:   true,
				Validators: []validator.String{stringvalidator.OneOf("basic", "standard", "customized")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ecdSimpleOfficeSiteResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).customEcdClient
}

// Create a new ECD Simple Office Site resource.
func (r *ecdSimpleOfficeSiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *ECDBasicOfficeSiteModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build request payload
	params := map[string]string{
		"CidrBlock": plan.CidrBlock.ValueString(),
	}

	if !plan.VpcId.IsNull() && !plan.VpcId.IsUnknown() {
		vpcId := plan.VpcId.ValueString()

		// Check if a simple office site already exists in this VPC.
		// Only 1 simple office site is allowed per VPC.
		existingResp, err := r.client.DescribeOfficeSitesByVpc(vpcId)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to Describe Office Sites by VPC.",
				err.Error(),
			)
			return
		}
		if existingId, found := ExtractOfficeSiteIdByVpc(existingResp, vpcId); found {
			resp.Diagnostics.AddError(
				"[API ERROR] Simple Office Site Already Exists.",
				fmt.Sprintf("VPC %s already has a simple office site. Only 1 per VPC is allowed. Existing ID: %s", vpcId, existingId),
			)
			return
		}

		params["VpcId"] = vpcId
	}
	if !plan.OfficeSiteName.IsNull() && !plan.OfficeSiteName.IsUnknown() {
		params["OfficeSiteName"] = plan.OfficeSiteName.ValueString()
	}
	if !plan.DesktopAccessType.IsNull() && !plan.DesktopAccessType.IsUnknown() {
		params["DesktopAccessType"] = plan.DesktopAccessType.ValueString()
	}
	if !plan.EnableAdminAccess.IsNull() && !plan.EnableAdminAccess.IsUnknown() {
		params["EnableAdminAccess"] = boolToString(plan.EnableAdminAccess.ValueBool())
	}
	if !plan.EnableCrossDesktopAccess.IsNull() && !plan.EnableCrossDesktopAccess.IsUnknown() {
		params["EnableCrossDesktopAccess"] = boolToString(plan.EnableCrossDesktopAccess.ValueBool())
	}
	if !plan.EnableInternetAccess.IsNull() && !plan.EnableInternetAccess.IsUnknown() {
		params["EnableInternetAccess"] = boolToString(plan.EnableInternetAccess.ValueBool())
	}
	if !plan.MfaEnabled.IsNull() && !plan.MfaEnabled.IsUnknown() {
		params["MfaEnabled"] = boolToString(plan.MfaEnabled.ValueBool())
	}
	if !plan.SsoEnabled.IsNull() && !plan.SsoEnabled.IsUnknown() {
		params["SsoEnabled"] = boolToString(plan.SsoEnabled.ValueBool())
	}
	if !plan.VpcType.IsNull() && !plan.VpcType.IsUnknown() {
		params["VpcType"] = plan.VpcType.ValueString()
	}
	if !plan.CenId.IsNull() && !plan.CenId.IsUnknown() {
		params["CenId"] = plan.CenId.ValueString()
	}
	if !plan.CenOwnerId.IsNull() && !plan.CenOwnerId.IsUnknown() {
		params["CenOwnerId"] = plan.CenOwnerId.ValueString()
	}
	if !plan.Bandwidth.IsNull() && !plan.Bandwidth.IsUnknown() {
		params["Bandwidth"] = fmt.Sprintf("%d", plan.Bandwidth.ValueInt64())
	}

	id, err := r.client.CreateSimpleOfficeSite(params)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create Simple Office Site.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &ECDBasicOfficeSiteModel{}
	state.Id = types.StringValue(id)
	state.VpcId = plan.VpcId
	state.CidrBlock = plan.CidrBlock
	state.CenId = plan.CenId
	state.CenOwnerId = plan.CenOwnerId
	state.OfficeSiteName = plan.OfficeSiteName
	state.DesktopAccessType = plan.DesktopAccessType
	state.EnableAdminAccess = plan.EnableAdminAccess
	state.EnableCrossDesktopAccess = plan.EnableCrossDesktopAccess
	state.EnableInternetAccess = plan.EnableInternetAccess
	state.MfaEnabled = plan.MfaEnabled
	state.SsoEnabled = plan.SsoEnabled
	state.VpcType = plan.VpcType
	state.Bandwidth = plan.Bandwidth

	// Resolve Unknown computed fields without overwriting values the user explicitly set.
	resolveUnknownFields(state, r.waitForOfficeSiteRegistered(id))

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read ECD Simple Office Site resource information.
func (r *ecdSimpleOfficeSiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from state
	var state *ECDBasicOfficeSiteModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	site, err := r.client.DescribeOfficeSiteById(state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe Simple Office Site.",
			err.Error(),
		)
		return
	}
	if site == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	populateComputedFields(state, site)

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the ECD Simple Office Site resource and sets the updated Terraform state on success.
func (r *ecdSimpleOfficeSiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *ECDBasicOfficeSiteModel

	// Retrieve values from plan
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state *ECDBasicOfficeSiteModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := map[string]string{}
	if !plan.OfficeSiteName.Equal(state.OfficeSiteName) {
		params["OfficeSiteName"] = plan.OfficeSiteName.ValueString()
	}
	if !plan.DesktopAccessType.Equal(state.DesktopAccessType) {
		params["DesktopAccessType"] = plan.DesktopAccessType.ValueString()
	}
	if !plan.MfaEnabled.Equal(state.MfaEnabled) {
		params["MfaEnabled"] = boolToString(plan.MfaEnabled.ValueBool())
	}
	if !plan.SsoEnabled.Equal(state.SsoEnabled) {
		params["SsoEnabled"] = boolToString(plan.SsoEnabled.ValueBool())
	}
	if !plan.EnableCrossDesktopAccess.Equal(state.EnableCrossDesktopAccess) {
		params["EnableCrossDesktopAccess"] = boolToString(plan.EnableCrossDesktopAccess.ValueBool())
	}

	if len(params) > 0 {
		if err := r.client.ModifyOfficeSiteAttribute(state.Id.ValueString(), params); err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to Modify Simple Office Site Attribute.",
				err.Error(),
			)
			return
		}
	}

	// Set state items
	newState := &ECDBasicOfficeSiteModel{}
	newState.Id = state.Id
	newState.VpcId = plan.VpcId
	newState.CidrBlock = plan.CidrBlock
	newState.CenId = plan.CenId
	newState.CenOwnerId = plan.CenOwnerId
	newState.OfficeSiteName = plan.OfficeSiteName
	newState.DesktopAccessType = plan.DesktopAccessType
	newState.EnableAdminAccess = plan.EnableAdminAccess
	newState.EnableCrossDesktopAccess = plan.EnableCrossDesktopAccess
	newState.EnableInternetAccess = plan.EnableInternetAccess
	newState.MfaEnabled = plan.MfaEnabled
	newState.SsoEnabled = plan.SsoEnabled
	newState.VpcType = plan.VpcType
	newState.Bandwidth = plan.Bandwidth

	// Resolve any Unknown computed fields from the API so Terraform has known values after apply.
	resolveUnknownFields(newState, r.waitForOfficeSiteRegistered(state.Id.ValueString()))

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &newState)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete the ECD Simple Office Site resource and removes the Terraform state on success.
func (r *ecdSimpleOfficeSiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *ECDBasicOfficeSiteModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteOfficeSite(state.Id.ValueString()); err != nil && !IsNotFoundError(err) {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Delete Simple Office Site.",
			err.Error(),
		)
		return
	}

	for attempt := 0; attempt < 18; attempt++ {
		site, _ := r.client.DescribeOfficeSiteById(state.Id.ValueString())
		if site == nil {
			break
		}
		time.Sleep(10 * time.Second)
	}
}

func (r *ecdSimpleOfficeSiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "InvalidOfficeSiteId") ||
		strings.Contains(err.Error(), "not exist")
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolVal(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func int64Val(m map[string]interface{}, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

// waitForOfficeSiteRegistered polls until the office site reaches REGISTERED
// status or the attempt limit is exhausted. Returns the last fetched site map.
func (r *ecdSimpleOfficeSiteResource) waitForOfficeSiteRegistered(id string) map[string]interface{} {
	var site map[string]interface{}
	for attempt := 0; attempt < 12; attempt++ {
		site, _ = r.client.DescribeOfficeSiteById(id)
		if site != nil && strVal(site, "Status") == "REGISTERED" {
			break
		}
		time.Sleep(10 * time.Second)
	}
	return site
}

// populateComputedFields fully refreshes all fields from the API response.
// Used by Read to sync current remote state.
func populateComputedFields(model *ECDBasicOfficeSiteModel, site map[string]interface{}) {
	model.Status = types.StringValue(strVal(site, "Status"))
	// Only overwrite OfficeSiteName if the API returns a non-empty value.
	// The API can return "" immediately after creation even though a name was set.
	if name := strVal(site, "OfficeSiteName"); name != "" {
		model.OfficeSiteName = types.StringValue(name)
	}
	model.VpcType = types.StringValue(strVal(site, "VpcType"))
	model.DesktopAccessType = types.StringValue(normalizeDesktopAccessType(strVal(site, "DesktopAccessType")))
	model.EnableAdminAccess = types.BoolValue(boolVal(site, "EnableAdminAccess"))
	model.EnableCrossDesktopAccess = types.BoolValue(boolVal(site, "EnableCrossDesktopAccess"))
	model.EnableInternetAccess = types.BoolValue(boolVal(site, "EnableInternetAccess"))
	model.MfaEnabled = types.BoolValue(boolVal(site, "MfaEnabled"))
	model.SsoEnabled = types.BoolValue(boolVal(site, "SsoEnabled"))
	model.Bandwidth = types.Int64Value(int64Val(site, "Bandwidth"))
}

// resolveUnknownFields is used after Create/Update: only sets fields that are still Unknown
// (i.e. Computed fields the user did not provide). User-set values are never overwritten.
func resolveUnknownFields(model *ECDBasicOfficeSiteModel, site map[string]interface{}) {
	apiVal := func(key string) string {
		if site != nil {
			return strVal(site, key)
		}
		return ""
	}
	apiBool := func(key string) bool {
		if site != nil {
			return boolVal(site, key)
		}
		return false
	}
	apiInt64 := func(key string) int64 {
		if site != nil {
			return int64Val(site, key)
		}
		return 0
	}

	// status is Computed-only — always resolve.
	model.Status = types.StringValue(apiVal("Status"))

	if model.OfficeSiteName.IsUnknown() {
		model.OfficeSiteName = types.StringValue(apiVal("OfficeSiteName"))
	}
	if model.VpcType.IsUnknown() {
		model.VpcType = types.StringValue(apiVal("VpcType"))
	}
	if model.DesktopAccessType.IsUnknown() {
		model.DesktopAccessType = types.StringValue(normalizeDesktopAccessType(apiVal("DesktopAccessType")))
	}
	if model.EnableAdminAccess.IsUnknown() {
		model.EnableAdminAccess = types.BoolValue(apiBool("EnableAdminAccess"))
	}
	if model.EnableCrossDesktopAccess.IsUnknown() {
		model.EnableCrossDesktopAccess = types.BoolValue(apiBool("EnableCrossDesktopAccess"))
	}
	if model.EnableInternetAccess.IsUnknown() {
		model.EnableInternetAccess = types.BoolValue(apiBool("EnableInternetAccess"))
	}
	if model.MfaEnabled.IsUnknown() {
		model.MfaEnabled = types.BoolValue(apiBool("MfaEnabled"))
	}
	if model.SsoEnabled.IsUnknown() {
		model.SsoEnabled = types.BoolValue(apiBool("SsoEnabled"))
	}
	if model.Bandwidth.IsUnknown() {
		model.Bandwidth = types.Int64Value(apiInt64("Bandwidth"))
	}
}

// normalizeDesktopAccessType converts API-returned uppercase values to the
// title-case format expected by the schema validator ("Any", "Internet", "VPC").
func normalizeDesktopAccessType(v string) string {
	switch strings.ToUpper(v) {
	case "INTERNET":
		return "Internet"
	case "VPC":
		return "VPC"
	case "ANY":
		return "Any"
	}
	return v
}
