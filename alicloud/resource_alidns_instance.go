package alicloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudDnsClient "github.com/alibabacloud-go/alidns-20150109/v4/client"
	alicloudBaseClient "github.com/alibabacloud-go/bssopenapi-20171214/v3/client"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ resource.Resource              = &alidnsInstanceResource{}
	_ resource.ResourceWithConfigure = &alidnsInstanceResource{}
)

func NewAlidnsInstanceResource() resource.Resource {
	return &alidnsInstanceResource{}
}

type alidnsInstanceResource struct {
	baseClient *alicloudBaseClient.Client
	client     *alicloudDnsClient.Client
}

type alidnsInstanceResourceModel struct {
	DnsSecurity   types.String `tfsdk:"dns_security"`
	DomainNumbers types.Int64  `tfsdk:"domain_numbers"`
	InstanceId    types.String `tfsdk:"instance_id"`
	PaymentType   types.String `tfsdk:"payment_type"`
	Period        types.Int64  `tfsdk:"period"`
	RenewPeriod   types.Int64  `tfsdk:"renew_period"`
	RenewalStatus types.String `tfsdk:"renewal_status"`
	VersionCode   types.String `tfsdk:"version_code"`
}

func (r *alidnsInstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alidns_instance"
}

func (r *alidnsInstanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"dns_security": schema.StringAttribute{
				Description: "Alidns instance security level." +
					"Valid value: no, basic, advanced.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("no", "basic", "advanced"),
				},
			},
			"domain_numbers": schema.Int64Attribute{
				Description: "Number of domain names bound.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.AtMost(100),
				},
			},
			"instance_id": schema.StringAttribute{
				Description: "Instance Domain Id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"payment_type": schema.StringAttribute{
				Description: "The Payment Type of the Global Traffic Manager instance." +
					"Valid value: Subscription.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("Subscription"),
				},
			},
			"period": schema.Int64Attribute{
				Description: "Creating a pre-paid instance, it must be set, the unit is month," +
					"please enter an integer multiple of 12 for annually paid products.",
				Required: true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AtMost(12),
				},
			},
			"renew_period": schema.Int64Attribute{
				Description: "Automatic renewal period, the unit is month. When setting RenewalStatus to AutoRenewal, it must be set.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AtMost(12),
				},
				Default: int64default.StaticInt64(0),
			},
			"renewal_status": schema.StringAttribute{
				Description: "Automatic renewal status. Valid values: AutoRenewal, ManualRenewal, default to ManualRenewal.",
				Computed:    true,
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("AutoRenewal", "ManualRenewal"),
				},
				Default: stringdefault.StaticString("ManualRenewal"),
			},
			"version_code": schema.StringAttribute{
				Description: "Paid package version. Valid values: version_personal, version_enterprise_basic, version_enterprise_advanced.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("version_personal", "version_enterprise_basic", "version_enterprise_advanced"),
				},
			},
		},
	}
}

func (r *alidnsInstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.baseClient = req.ProviderData.(alicloudClients).baseClient
	r.client = req.ProviderData.(alicloudClients).dnsClient
}

func (r *alidnsInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan, state *alidnsInstanceResourceModel
	planDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(planDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createAlidnsInstanceRequest := &alicloudBaseClient.CreateInstanceRequest{
		ProductCode:      tea.String("dns"),
		ProductType:      tea.String("dns_dns_public_intl"),
		SubscriptionType: tea.String(plan.PaymentType.ValueString()),
		Period:           tea.Int32(int32(plan.Period.ValueInt64())),
		RenewalStatus:    tea.String(plan.RenewalStatus.ValueString()),
	}
	createAlidnsInstanceRequest.Parameter = []*alicloudBaseClient.CreateInstanceRequestParameter{
		{
			Code:  tea.String("InstanceType"),
			Value: tea.String("HostedPublicZone"),
		},
		{
			Code:  tea.String("Version"),
			Value: tea.String(plan.VersionCode.ValueString()),
		},
		{
			Code:  tea.String("DNSSecurity"),
			Value: tea.String(plan.DnsSecurity.ValueString()),
		},
		{
			Code:  tea.String("DomainNumbers"),
			Value: tea.String(fmt.Sprintf("%d", plan.DomainNumbers.ValueInt64())),
		},
	}

	if plan.RenewalStatus.ValueString() == "AutoRenewal" && plan.RenewPeriod.IsNull() {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create AliDNS Instance",
			"renew_period is required when AutoRenewal is set in renewal_status.",
		)
		return
	} else {
		createAlidnsInstanceRequest.RenewPeriod = tea.Int32(int32(plan.RenewPeriod.ValueInt64()))
	}

	createInstanceResponse := &alicloudBaseClient.CreateInstanceResponse{}
	var err error
	createAlidnsInstance := func() error {
		runtime := &util.RuntimeOptions{}
		if createInstanceResponse, err = r.baseClient.CreateInstanceWithOptions(createAlidnsInstanceRequest, runtime); err != nil {
			if _t, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*_t.Code) {
					return err
				} else if *_t.Code == "NotApplicable" {
					r.baseClient.Endpoint = tea.String("business.ap-southeast-1.aliyuncs.com")
					return err
				} else {
					return backoff.Permanent(err)
				}
			} else {
				return err
			}
		}

		if *createInstanceResponse.Body.Code == "PAY.AMOUNT_LIMIT_EXCEEDED" {
			return backoff.Permanent(fmt.Errorf("%s", createInstanceResponse.String()))
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(createAlidnsInstance, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create AliDNS Instance",
			err.Error(),
		)
		return
	}

	state = &alidnsInstanceResourceModel{}
	state = plan
	state.InstanceId = types.StringValue(*createInstanceResponse.Body.Data.InstanceId)
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *alidnsInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *alidnsInstanceResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var describeRsp *alicloudDnsClient.DescribeDnsProductInstanceResponse
	var queryRsp *alicloudBaseClient.QueryAvailableInstancesResponse
	readInstanceDomain := func() (err error) {
		runtime := &util.RuntimeOptions{}

		describeDnsProductInstanceRequest := &alicloudDnsClient.DescribeDnsProductInstanceRequest{
			InstanceId: tea.String(state.InstanceId.ValueString()),
		}
		if describeRsp, err = r.client.DescribeDnsProductInstanceWithOptions(describeDnsProductInstanceRequest, runtime); err != nil {
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

		queryAvailableInstanceRequest := &alicloudBaseClient.QueryAvailableInstancesRequest{
			InstanceIDs: tea.String(state.InstanceId.ValueString()),
		}
		if queryRsp, err = r.baseClient.QueryAvailableInstancesWithOptions(queryAvailableInstanceRequest, runtime); err != nil {
			if _t, ok := err.(*tea.SDKError); ok {
				if *_t.Code == "NotApplicable" {
					r.baseClient.Endpoint = tea.String("business.ap-southeast-1.aliyuncs.com")
					return err
				} else if isAbleToRetry(*_t.Code) {
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
	err := backoff.Retry(readInstanceDomain, reconnectBackoff)
	if err != nil {
		// Remove state if dns instance is not found
		// This will make terraform to create a new instance
		if strings.Contains(err.Error(), "InvalidDnsProduct") {
			resp.State.RemoveResource(ctx)
			return
		} else {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to find DNS Instance.",
				err.Error(),
			)
		}
		return
	}

	switch *describeRsp.Body.DnsSecurity {
	case "Not Required":
		state.DnsSecurity = types.StringValue("no")
	case "DNS Attack Defense Basic":
		state.DnsSecurity = types.StringValue("basic")
	case "DNS Anti-DDoS Basic":
		state.DnsSecurity = types.StringValue("basic")
	case "DNS Anti-DDoS Advanced":
		state.DnsSecurity = types.StringValue("advanced")
	}
	state.DomainNumbers = types.Int64Value(*describeRsp.Body.BindDomainCount)
	state.PaymentType = types.StringValue(*describeRsp.Body.PaymentType)
	if queryRsp.Body.Data.InstanceList[0].RenewalDuration == nil {
		state.RenewPeriod = types.Int64Value(0)
	} else {
		state.RenewPeriod = types.Int64Value(int64(*queryRsp.Body.Data.InstanceList[0].RenewalDuration))
	}
	state.RenewalStatus = types.StringValue(*queryRsp.Body.Data.InstanceList[0].RenewStatus)
	state.VersionCode = types.StringValue(*describeRsp.Body.VersionCode)

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *alidnsInstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan, state *alidnsInstanceResourceModel
	planDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(planDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	modifyAlidnsInstanceRequest := &alicloudBaseClient.ModifyInstanceRequest{
		ProductCode:      tea.String("dns"),
		ProductType:      tea.String("dns_dns_public_intl"),
		ModifyType:       tea.String("Upgrade"),
		InstanceId:       tea.String(state.InstanceId.ValueString()),
		SubscriptionType: tea.String(state.PaymentType.ValueString()),
	}

	modifyAlidnsInstanceRequest.Parameter = []*alicloudBaseClient.ModifyInstanceRequestParameter{
		{
			Code:  tea.String("Version"),
			Value: tea.String(plan.VersionCode.ValueString()),
		},
		{
			Code:  tea.String("DNSSecurity"),
			Value: tea.String(plan.DnsSecurity.ValueString()),
		},
		{
			Code:  tea.String("DomainNumbers"),
			Value: tea.String(fmt.Sprintf("%d", plan.DomainNumbers.ValueInt64())),
		},
	}

	//////////////////////// DATA VALIDATION ////////////////////////
	// Make sure no downgrade action is perform as downgrade is not supported.
	// In order to downgrade, state remove the old instance and apply a new one.
	if plan.DnsSecurity.ValueString() == "no" && state.DnsSecurity.ValueString() != "no" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Upgrade AliDNS Instance",
			"Downgrading of AliDNS Instance's DNS Protection is not supported.",
		)
		return
	}
	if plan.DnsSecurity.ValueString() == "basic" && state.DnsSecurity.ValueString() == "advanced" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Upgrade AliDNS Instance",
			"Downgrading of AliDNS Instance's DNS Protection is not supported.",
		)
		return
	}
	if plan.VersionCode.ValueString() == "version_personal" && state.VersionCode.ValueString() != "version_personal" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Upgrade AliDNS Instance",
			"Downgrading of AliDNS Instance's Edition is not supported.",
		)
		return
	}
	if plan.VersionCode.ValueString() == "version_enterprise_basic" && state.VersionCode.ValueString() == "version_enterprise_advanced" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Upgrade AliDNS Instance",
			"Downgrading of AliDNS Instance's Edition is not supported.",
		)
		return
	}
	if plan.Period != state.Period {
		resp.Diagnostics.AddWarning(
			"[Input Warning] Changing period have no effect",
			"Changing period have no effect to the instance once the instance is built.",
		)
	}

	//////////////////////// UPGRADE INSTANCE ////////////////////////
	// Modify renewal status if changes detected
	setRenewalRequest := &alicloudBaseClient.SetRenewalRequest{
		InstanceIDs:   tea.String(state.InstanceId.ValueString()),
		RenewalStatus: tea.String(plan.RenewalStatus.ValueString()),
		ProductCode:   tea.String("dns"),
		ProductType:   tea.String("dns_dns_public_intl"),
	}
	var err error
	err = r.setInstanceRenewal(setRenewalRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Disable DNS Instance Renewal",
			fmt.Sprint(state),
		)
	}

	modifyInstanceResponse := &alicloudBaseClient.ModifyInstanceResponse{}
	modifyAlidnsInstance := func() error {
		runtime := &util.RuntimeOptions{}
		if modifyInstanceResponse, err = r.baseClient.ModifyInstanceWithOptions(modifyAlidnsInstanceRequest, runtime); err != nil {
			if _t, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*_t.Code) {
					return err
				} else if *_t.Code == "NotApplicable" {
					r.baseClient.Endpoint = tea.String("business.ap-southeast-1.aliyuncs.com")
					return err
				} else {
					return backoff.Permanent(err)
				}
			} else {
				return err
			}
		}

		if *modifyInstanceResponse.Body.Code == "PAY.AMOUNT_LIMIT_EXCEEDED" ||
			*modifyInstanceResponse.Body.Code == "MissingParameter" {
			return backoff.Permanent(fmt.Errorf("%s", modifyInstanceResponse.String()))
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(modifyAlidnsInstance, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create AliDNS Instance",
			err.Error(),
		)
		return
	}

	state.PaymentType = plan.PaymentType
	state.DnsSecurity = plan.DnsSecurity
	state.DomainNumbers = plan.DomainNumbers
	state.VersionCode = plan.VersionCode
	state.RenewPeriod = plan.RenewPeriod
	state.RenewalStatus = plan.RenewalStatus
	state.Period = plan.Period
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *alidnsInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *alidnsInstanceResourceModel
	planDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(planDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	setRenewalRequest := &alicloudBaseClient.SetRenewalRequest{
		InstanceIDs:   tea.String(state.InstanceId.ValueString()),
		RenewalStatus: tea.String("NotRenewal"),
		ProductCode:   tea.String("dns"),
		ProductType:   tea.String("dns_dns_public_intl"),
	}

	err := r.setInstanceRenewal(setRenewalRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Disable DNS Instance Renewal",
			fmt.Sprint(state),
		)
	}
}

func (r *alidnsInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("instance_id"), req, resp)
}

func (r alidnsInstanceResource) setInstanceRenewal(req *alicloudBaseClient.SetRenewalRequest) error {
	setRenewal := func() error {
		runtime := &util.RuntimeOptions{}
		_, err := r.baseClient.SetRenewalWithOptions(req, runtime)
		if err != nil {
			if _t, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*_t.Code) {
					return err
				} else if *_t.Code == "NotApplicable" {
					r.baseClient.Endpoint = tea.String("business.ap-southeast-1.aliyuncs.com")
					return err
				} else {
					return backoff.Permanent(err)
				}
			}
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	return backoff.Retry(setRenewal, reconnectBackoff)
}
