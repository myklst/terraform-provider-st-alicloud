package alicloud

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cenkalti/backoff"

	alicloudEcdClient "github.com/alibabacloud-go/ecd-20200930/v5/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ecdDesktopResource{}
	_ resource.ResourceWithConfigure   = &ecdDesktopResource{}
	_ resource.ResourceWithImportState = &ecdDesktopResource{}
)

func NewAliecdDesktopResource() resource.Resource {
	return &ecdDesktopResource{}
}

type ecdDesktopResource struct {
	client *alicloudEcdClient.Client
}

type ecdDesktopModel struct {
	Id             types.String `tfsdk:"id"`
	OfficeSiteId   types.String `tfsdk:"office_site_id"`
	BundleId       types.String `tfsdk:"bundle_id"`
	PolicyGroupId  types.String `tfsdk:"policy_group_id"`
	VSwitchId      types.String `tfsdk:"vswitch_id"`
	DesktopName    types.String `tfsdk:"desktop_name"`
	DesktopType    types.String `tfsdk:"desktop_type"`
	HostName       types.String `tfsdk:"host_name"`
	PaymentType    types.String `tfsdk:"payment_type"`
	UserAssignMode types.String `tfsdk:"user_assign_mode"`
	EndUserIds     types.List   `tfsdk:"end_user_ids"`
	Tags           types.Map    `tfsdk:"tags"`
	StoppedMode    types.String `tfsdk:"stopped_mode"`
	AutoPay        types.Bool   `tfsdk:"auto_pay"`
	AutoRenew      types.Bool   `tfsdk:"auto_renew"`
	Period         types.Int64  `tfsdk:"period"`
	PeriodUnit     types.String `tfsdk:"period_unit"`
	Status         types.String `tfsdk:"status"`
}

// Metadata returns the ECD desktop resource type name.
func (r *ecdDesktopResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ecd_desktop"
}

func (r *ecdDesktopResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Alicloud ECD Desktop resource. Creates a virtual desktop inside a Simple Office Site.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the ECD Desktop.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"office_site_id": schema.StringAttribute{
				Description: "The ID of the Simple Office Site in which to create the desktop.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bundle_id": schema.StringAttribute{
				Description: "The ID of the desktop bundle (defines OS image and hardware spec).",
				Required:    true,
			},
			"policy_group_id": schema.StringAttribute{
				Description: "The ID of the desktop policy to apply.",
				Required:    true,
			},
			"vswitch_id": schema.StringAttribute{
				Description: "The ID of the VSwitch in which to deploy the desktop. Not required for Simple Office Sites.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"desktop_name": schema.StringAttribute{
				Description: "The name of the desktop.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"desktop_type": schema.StringAttribute{
				Description: "The type of the desktop. Determined by the bundle.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"host_name": schema.StringAttribute{
				Description: "The hostname of the desktop. Valid only for AD (Enterprise) office networks with Windows desktops. Do not set this for Simple Office Sites.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"payment_type": schema.StringAttribute{
				Description: "The billing method of the desktop. Valid values: PayAsYouGo, Subscription.",
				Optional:    true,
				Computed:    true,
				Validators:  []validator.String{stringvalidator.OneOf("PayAsYouGo", "Subscription")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_assign_mode": schema.StringAttribute{
				Description: "The user assignment mode. Valid values: ALL, PER_USER.",
				Optional:    true,
				Validators:  []validator.String{stringvalidator.OneOf("ALL", "PER_USER")},
			},
			"end_user_ids": schema.ListAttribute{
				Description: "The IDs of the end users to assign to the desktop.",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"auto_pay": schema.BoolAttribute{
				Description: "Whether to enable automatic payment for Subscription desktops.",
				Optional:    true,
			},
			"auto_renew": schema.BoolAttribute{
				Description: "Whether to enable automatic renewal for Subscription desktops.",
				Optional:    true,
			},
			"period": schema.Int64Attribute{
				Description: "The subscription duration. Used when payment_type is Subscription.",
				Optional:    true,
			},
			"period_unit": schema.StringAttribute{
				Description: "The unit of the subscription duration. Valid values: Month, Year.",
				Optional:    true,
				Validators:  []validator.String{stringvalidator.OneOf("Month", "Year")},
			},
			"stopped_mode": schema.StringAttribute{
				Description: "The stopped mode of the desktop. Valid values: StopCharging, KeepCharging. Applied when the desktop is stopped.",
				Optional:    true,
				Validators:  []validator.String{stringvalidator.OneOf("StopCharging", "KeepCharging")},
			},
			"tags": schema.MapAttribute{
				Description: "A map of tags to assign to the desktop.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"status": schema.StringAttribute{
				Description: "The current status of the desktop.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ecdDesktopResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).ecdClient
}

// Create a new ECD desktop resource.
func (r *ecdDesktopResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *ecdDesktopModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	desktopId, err := r.createDesktop(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create ECD Desktop.",
			err.Error(),
		)
		return
	}

	desktop, err := r.waitForDesktopRunning(desktopId)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe ECD Desktop.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &ecdDesktopModel{}
	state.Id = types.StringValue(desktopId)
	state.OfficeSiteId = plan.OfficeSiteId
	state.BundleId = plan.BundleId
	state.PolicyGroupId = plan.PolicyGroupId
	state.VSwitchId = plan.VSwitchId
	state.DesktopName = plan.DesktopName
	state.HostName = plan.HostName
	state.PaymentType = plan.PaymentType
	state.UserAssignMode = plan.UserAssignMode
	state.EndUserIds = plan.EndUserIds
	state.Tags = plan.Tags
	state.StoppedMode = plan.StoppedMode
	state.AutoPay = plan.AutoPay
	state.AutoRenew = plan.AutoRenew
	state.Period = plan.Period
	state.PeriodUnit = plan.PeriodUnit
	state.DesktopType = types.StringValue("")
	state.Status = types.StringValue("")

	if desktop != nil {
		state.Status = types.StringValue(tea.StringValue(desktop.DesktopStatus))
		state.DesktopType = types.StringValue(tea.StringValue(desktop.DesktopType))
		if state.DesktopName.IsUnknown() {
			state.DesktopName = types.StringValue(tea.StringValue(desktop.DesktopName))
		}
		if state.HostName.IsUnknown() {
			state.HostName = types.StringValue(tea.StringValue(desktop.HostName))
		}
		if state.PaymentType.IsUnknown() {
			paymentType := "PayAsYouGo"
			if tea.StringValue(desktop.ChargeType) == "PrePaid" {
				paymentType = "Subscription"
			}
			state.PaymentType = types.StringValue(paymentType)
		}
		if state.EndUserIds.IsNull() || state.EndUserIds.IsUnknown() {
			state.EndUserIds = flattenStringList(desktop.EndUserIds)
		}
		state.Tags = flattenTags(desktop.Tags)
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read ECD desktop resource information.
func (r *ecdDesktopResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from state
	var state *ecdDesktopModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	desktop, err := r.describeDesktopById(state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe ECD Desktop.",
			err.Error(),
		)
		return
	}
	if desktop == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// VSwitchId, BundleId are not returned by DescribeDesktops — keep values from state.
	// auto_pay, auto_renew, period, period_unit are write-only — keep from state.
	state.Status = types.StringValue(tea.StringValue(desktop.DesktopStatus))
	state.OfficeSiteId = types.StringValue(tea.StringValue(desktop.OfficeSiteId))
	// BundleId: never overwrite from API — keep exactly what is in state.
	state.PolicyGroupId = types.StringValue(tea.StringValue(desktop.PolicyGroupId))
	state.DesktopType = types.StringValue(tea.StringValue(desktop.DesktopType))
	paymentType := "PayAsYouGo"
	if tea.StringValue(desktop.ChargeType) == "PrePaid" {
		paymentType = "Subscription"
	}
	state.PaymentType = types.StringValue(paymentType)
	state.EndUserIds = flattenStringList(desktop.EndUserIds)
	state.Tags = flattenTags(desktop.Tags)
	if name := tea.StringValue(desktop.DesktopName); name != "" {
		state.DesktopName = types.StringValue(name)
	}
	if hostName := tea.StringValue(desktop.HostName); hostName != "" {
		state.HostName = types.StringValue(hostName)
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the ECD desktop resource and sets the updated Terraform state on success.
func (r *ecdDesktopResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *ecdDesktopModel

	// Retrieve values from plan
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state *ecdDesktopModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.DesktopName.Equal(state.DesktopName) {
		if err := r.modifyDesktopName(state.Id.ValueString(), plan.DesktopName.ValueString()); err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to Modify ECD Desktop Name.",
				err.Error(),
			)
			return
		}
	}

	if !plan.PolicyGroupId.Equal(state.PolicyGroupId) {
		if err := r.modifyDesktopPolicyGroup(state.Id.ValueString(), plan.PolicyGroupId.ValueString()); err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to Modify ECD Desktop Policy Group.",
				err.Error(),
			)
			return
		}
	}

	if !plan.Tags.Equal(state.Tags) {
		if err := r.syncTags(ctx, state.Id.ValueString(), state.Tags, plan.Tags); err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to Update ECD Desktop Tags.",
				err.Error(),
			)
			return
		}
	}

	desktop, err := r.describeDesktopById(state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe ECD Desktop.",
			err.Error(),
		)
		return
	}

	// Set state items
	newState := &ecdDesktopModel{}
	newState.Id = state.Id
	newState.OfficeSiteId = plan.OfficeSiteId
	newState.BundleId = plan.BundleId
	newState.PolicyGroupId = plan.PolicyGroupId
	newState.VSwitchId = plan.VSwitchId
	newState.DesktopName = plan.DesktopName
	newState.HostName = plan.HostName
	newState.PaymentType = plan.PaymentType
	newState.UserAssignMode = plan.UserAssignMode
	newState.EndUserIds = plan.EndUserIds
	newState.Tags = plan.Tags
	newState.StoppedMode = plan.StoppedMode
	newState.AutoPay = plan.AutoPay
	newState.AutoRenew = plan.AutoRenew
	newState.Period = plan.Period
	newState.PeriodUnit = plan.PeriodUnit
	newState.DesktopType = state.DesktopType
	newState.Status = state.Status

	if desktop != nil {
		newState.Status = types.StringValue(tea.StringValue(desktop.DesktopStatus))
		newState.DesktopType = types.StringValue(tea.StringValue(desktop.DesktopType))
		updatePaymentType := "PayAsYouGo"
		if tea.StringValue(desktop.ChargeType) == "PrePaid" {
			updatePaymentType = "Subscription"
		}
		newState.PaymentType = types.StringValue(updatePaymentType)
		newState.Tags = flattenTags(desktop.Tags)
		if name := tea.StringValue(desktop.DesktopName); name != "" {
			newState.DesktopName = types.StringValue(name)
		}
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &newState)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete the ECD desktop resource and removes the Terraform state on success.
func (r *ecdDesktopResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *ecdDesktopModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.deleteDesktop(state.Id.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Delete ECD Desktop.",
			err.Error(),
		)
		return
	}

	// Wait for the desktop to be fully removed so dependent resources
	// (e.g. the office site) can be deleted safely afterward.
	for attempt := 0; attempt < 18; attempt++ {
		desktop, _ := r.describeDesktopById(state.Id.ValueString())
		if desktop == nil {
			break
		}
		time.Sleep(10 * time.Second)
	}
}

func (r *ecdDesktopResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ecdDesktopResource) createDesktop(ctx context.Context, plan *ecdDesktopModel) (string, error) {
	var desktopId string

	amount := int32(1)
	createDesktopRequest := &alicloudEcdClient.CreateDesktopsRequest{
		OfficeSiteId:  tea.String(plan.OfficeSiteId.ValueString()),
		BundleId:      tea.String(plan.BundleId.ValueString()),
		PolicyGroupId: tea.String(plan.PolicyGroupId.ValueString()),
		Amount:        &amount,
	}
	if !plan.VSwitchId.IsNull() && !plan.VSwitchId.IsUnknown() {
		createDesktopRequest.SubnetId = tea.String(plan.VSwitchId.ValueString())
	}
	if !plan.DesktopName.IsNull() && !plan.DesktopName.IsUnknown() {
		createDesktopRequest.DesktopName = tea.String(plan.DesktopName.ValueString())
	}
	if !plan.HostName.IsNull() && !plan.HostName.IsUnknown() {
		createDesktopRequest.Hostname = tea.String(plan.HostName.ValueString())
	}
	if !plan.PaymentType.IsNull() && !plan.PaymentType.IsUnknown() {
		chargeType := "PostPaid"
		if plan.PaymentType.ValueString() == "Subscription" {
			chargeType = "PrePaid"
		}
		createDesktopRequest.ChargeType = tea.String(chargeType)
	}
	if !plan.UserAssignMode.IsNull() && !plan.UserAssignMode.IsUnknown() {
		createDesktopRequest.UserAssignMode = tea.String(plan.UserAssignMode.ValueString())
	}
	if !plan.AutoPay.IsNull() && !plan.AutoPay.IsUnknown() {
		createDesktopRequest.AutoPay = tea.Bool(plan.AutoPay.ValueBool())
	}
	if !plan.AutoRenew.IsNull() && !plan.AutoRenew.IsUnknown() {
		createDesktopRequest.AutoRenew = tea.Bool(plan.AutoRenew.ValueBool())
	}
	if !plan.Period.IsNull() && !plan.Period.IsUnknown() {
		p := int32(plan.Period.ValueInt64())
		createDesktopRequest.Period = &p
	}
	if !plan.PeriodUnit.IsNull() && !plan.PeriodUnit.IsUnknown() {
		createDesktopRequest.PeriodUnit = tea.String(plan.PeriodUnit.ValueString())
	}
	if !plan.EndUserIds.IsNull() && !plan.EndUserIds.IsUnknown() {
		var ids []string
		plan.EndUserIds.ElementsAs(ctx, &ids, false)
		endUserIds := make([]*string, len(ids))
		for i, id := range ids {
			endUserIds[i] = tea.String(id)
		}
		createDesktopRequest.EndUserId = endUserIds
	}
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tagsMap map[string]string
		plan.Tags.ElementsAs(ctx, &tagsMap, false)
		sdkTags := make([]*alicloudEcdClient.CreateDesktopsRequestTag, 0, len(tagsMap))
		for k, v := range tagsMap {
			sdkTags = append(sdkTags, &alicloudEcdClient.CreateDesktopsRequestTag{
				Key:   tea.String(k),
				Value: tea.String(v),
			})
		}
		createDesktopRequest.Tag = sdkTags
	}

	createDesktop := func() error {
		runtime := &util.RuntimeOptions{}
		createResp, err := r.client.CreateDesktopsWithOptions(createDesktopRequest, runtime)
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
		if len(createResp.Body.DesktopId) == 0 {
			return backoff.Permanent(errors.New("CreateDesktops: no DesktopId returned"))
		}
		desktopId = tea.StringValue(createResp.Body.DesktopId[0])
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	if err := backoff.Retry(createDesktop, reconnectBackoff); err != nil {
		return "", err
	}
	return desktopId, nil
}

func (r *ecdDesktopResource) describeDesktopById(id string) (*alicloudEcdClient.DescribeDesktopsResponseBodyDesktops, error) {
	var desktop *alicloudEcdClient.DescribeDesktopsResponseBodyDesktops

	describeDesktop := func() error {
		runtime := &util.RuntimeOptions{}
		describeResp, err := r.client.DescribeDesktopsWithOptions(&alicloudEcdClient.DescribeDesktopsRequest{
			DesktopId: []*string{tea.String(id)},
		}, runtime)
		if err != nil {
			if isEcdDesktopNotFoundError(err) {
				return nil
			}
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
		if describeResp.Body != nil && len(describeResp.Body.Desktops) > 0 {
			desktop = describeResp.Body.Desktops[0]
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	if err := backoff.Retry(describeDesktop, reconnectBackoff); err != nil {
		return nil, err
	}
	return desktop, nil
}

func (r *ecdDesktopResource) modifyDesktopName(id, name string) error {
	modifyDesktopName := func() error {
		runtime := &util.RuntimeOptions{}
		_, err := r.client.ModifyDesktopNameWithOptions(&alicloudEcdClient.ModifyDesktopNameRequest{
			DesktopId:      tea.String(id),
			NewDesktopName: tea.String(name),
		}, runtime)
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
	return backoff.Retry(modifyDesktopName, reconnectBackoff)
}

func (r *ecdDesktopResource) modifyDesktopPolicyGroup(id, policyGroupId string) error {
	modifyPolicyGroup := func() error {
		runtime := &util.RuntimeOptions{}
		_, err := r.client.ModifyDesktopsPolicyGroupWithOptions(&alicloudEcdClient.ModifyDesktopsPolicyGroupRequest{
			DesktopId:     []*string{tea.String(id)},
			PolicyGroupId: tea.String(policyGroupId),
		}, runtime)
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
	return backoff.Retry(modifyPolicyGroup, reconnectBackoff)
}

func (r *ecdDesktopResource) deleteDesktop(id string) error {
	deleteDesktop := func() error {
		runtime := &util.RuntimeOptions{}
		_, err := r.client.DeleteDesktopsWithOptions(&alicloudEcdClient.DeleteDesktopsRequest{
			DesktopId: []*string{tea.String(id)},
		}, runtime)
		if err != nil {
			if isEcdDesktopNotFoundError(err) {
				return nil
			}
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
	return backoff.Retry(deleteDesktop, reconnectBackoff)
}

// waitForDesktopRunning polls until the desktop reaches Running status or the attempt limit is hit.
func (r *ecdDesktopResource) waitForDesktopRunning(id string) (*alicloudEcdClient.DescribeDesktopsResponseBodyDesktops, error) {
	var desktop *alicloudEcdClient.DescribeDesktopsResponseBodyDesktops
	var err error
	for attempt := 0; attempt < 18; attempt++ {
		desktop, err = r.describeDesktopById(id)
		if err != nil {
			return nil, err
		}
		if desktop != nil && tea.StringValue(desktop.DesktopStatus) == "Running" {
			break
		}
		time.Sleep(10 * time.Second)
	}
	return desktop, nil
}

// flattenStringList converts a []*string slice from the API into a types.List.
func flattenStringList(vals []*string) types.List {
	if len(vals) == 0 {
		return types.ListNull(types.StringType)
	}
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(tea.StringValue(v))
	}
	list, _ := types.ListValue(types.StringType, elems)
	return list
}

// flattenTags converts the API tag list into a types.Map.
func flattenTags(tags []*alicloudEcdClient.DescribeDesktopsResponseBodyDesktopsTags) types.Map {
	if len(tags) == 0 {
		return types.MapNull(types.StringType)
	}
	elems := make(map[string]attr.Value, len(tags))
	for _, tag := range tags {
		elems[tea.StringValue(tag.Key)] = types.StringValue(tea.StringValue(tag.Value))
	}
	m, _ := types.MapValue(types.StringType, elems)
	return m
}

// syncTags reconciles the desired tag state by adding new/changed tags and removing deleted ones.
func (r *ecdDesktopResource) syncTags(ctx context.Context, id string, stateTags, planTags types.Map) error {
	stateElems := map[string]string{}
	planElems := map[string]string{}
	if !stateTags.IsNull() && !stateTags.IsUnknown() {
		stateTags.ElementsAs(ctx, &stateElems, false)
	}
	if !planTags.IsNull() && !planTags.IsUnknown() {
		planTags.ElementsAs(ctx, &planElems, false)
	}

	var tagsToAdd []*alicloudEcdClient.TagResourcesRequestTag
	for k, v := range planElems {
		if stateVal, exists := stateElems[k]; !exists || stateVal != v {
			tagsToAdd = append(tagsToAdd, &alicloudEcdClient.TagResourcesRequestTag{
				Key:   tea.String(k),
				Value: tea.String(v),
			})
		}
	}

	var keysToRemove []*string
	for k := range stateElems {
		if _, exists := planElems[k]; !exists {
			keysToRemove = append(keysToRemove, tea.String(k))
		}
	}

	if len(tagsToAdd) > 0 {
		if err := r.tagResources(id, tagsToAdd); err != nil {
			return err
		}
	}
	if len(keysToRemove) > 0 {
		if err := r.untagResources(id, keysToRemove); err != nil {
			return err
		}
	}
	return nil
}

func (r *ecdDesktopResource) tagResources(id string, tags []*alicloudEcdClient.TagResourcesRequestTag) error {
	tagResources := func() error {
		runtime := &util.RuntimeOptions{}
		_, err := r.client.TagResourcesWithOptions(&alicloudEcdClient.TagResourcesRequest{
			ResourceId:   []*string{tea.String(id)},
			ResourceType: tea.String("desktops"),
			Tag:          tags,
		}, runtime)
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
	return backoff.Retry(tagResources, reconnectBackoff)
}

func (r *ecdDesktopResource) untagResources(id string, tagKeys []*string) error {
	untagResources := func() error {
		runtime := &util.RuntimeOptions{}
		_, err := r.client.UntagResourcesWithOptions(&alicloudEcdClient.UntagResourcesRequest{
			ResourceId:   []*string{tea.String(id)},
			ResourceType: tea.String("desktops"),
			TagKey:       tagKeys,
		}, runtime)
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
	return backoff.Retry(untagResources, reconnectBackoff)
}

// isEcdDesktopNotFoundError returns true when the error indicates the desktop does not exist.
func isEcdDesktopNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "InvalidDesktopId") || strings.Contains(msg, "not exist")
}
