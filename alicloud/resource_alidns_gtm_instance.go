package alicloud

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudDnsClient "github.com/alibabacloud-go/alidns-20150109/v4/client"
	alicloudBaseClient "github.com/alibabacloud-go/bssopenapi-20171214/v3/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

func getAlertConfigNoticeType() []string {
	return []string{
		"ADDR_RESUME",
		"ADDR_ALERT",
		"ADDR_POOL_GROUP_UNAVAILABLE",
		"ADDR_POOL_GROUP_AVAILABLE",
		"ACCESS_STRATEGY_POOL_GROUP_SWITCH",
		"MONITOR_NODE_IP_CHANGE",
	}
}

type CreateAlidnsGtmInstanceResponse struct {
	Headers    map[string]*string                   `json:"headers,omitempty" xml:"headers,omitempty" require:"true"`
	StatusCode *int32                               `json:"statusCode,omitempty" xml:"statusCode,omitempty" require:"true"`
	Body       *CreateAlidnsGtmInstanceResponseBody `json:"body,omitempty" xml:"body,omitempty" require:"true"`
}

type CreateAlidnsGtmInstanceResponseBody struct {
	// The details about the accelerated domain name.
	Code *string                                     `json:"Code,omitempty" xml:"Code,omitempty"`
	Data *CreateAlidnsGtmInstanceResponseBodyDetails `json:"Data,omitempty" xml:"Data,omitempty" type:"Struct"`
	// The ID of the request.
	RequestId *string `json:"RequestId,omitempty" xml:"RequestId,omitempty"`
}

type CreateAlidnsGtmInstanceResponseBodyDetails struct {
	InstanceId *string `json:"InstanceId,omitempty" xml:"InstanceId,omitempty"`
	OrderId    *string `json:"OrderId,omitempty" xml:"OrderId,omitempty"`
}

var (
	_ resource.Resource                = &alidnsGtmInstanceResource{}
	_ resource.ResourceWithConfigure   = &alidnsGtmInstanceResource{}
	_ resource.ResourceWithImportState = &alidnsGtmInstanceResource{}
	_ resource.ResourceWithModifyPlan  = &alidnsGtmInstanceResource{}
)

func NewAliDnsGtmInstanceResource() resource.Resource {
	return &alidnsGtmInstanceResource{}
}

type alidnsGtmInstanceResource struct {
	baseClient *alicloudBaseClient.Client
	client     *alicloudDnsClient.Client
}

type alidnsGtmInstanceResourceModel struct {
	// Required
	InstanceType   types.String `tfsdk:"instance_type"`
	InstanceName   types.String `tfsdk:"instance_name"`
	PaymentType    types.String `tfsdk:"payment_type"`
	PackageEdition types.String `tfsdk:"package_edition"`
	// HealthcheckTaskCount types.Int64    `tfsdk:"health_check_task_count"`
	Ttl             types.Int64    `tfsdk:"ttl"`
	AlertGroup      types.List     `tfsdk:"alert_group"`
	ResourceGroupID types.String   `tfsdk:"resource_group_id"`
	AlertConfig     []*alertConfig `tfsdk:"alert_config"`
	RenewPeriod     types.Int64    `tfsdk:"renew_period"`
	RenewalStatus   types.String   `tfsdk:"renewal_status"`

	// Optional
	Id          types.String `tfsdk:"id"`
	CnameType   types.String `tfsdk:"cname_type"`
	ForceUpdate types.Bool   `tfsdk:"force_update"`

	SmsNotificationCount types.Int64  `tfsdk:"sms_notification_count"`
	StrategyMode         types.String `tfsdk:"strategy_mode"`
	PublicCnameMode      types.String `tfsdk:"public_cname_mode"`
	PublicRr             types.String `tfsdk:"public_rr"`
	PublicUserDomainName types.String `tfsdk:"public_user_domain_name"`
	PublicZoneName       types.String `tfsdk:"public_zone_name"`
}

type alertConfig struct {
	DingtalkNotice types.Bool   `tfsdk:"dingtalk_notice"`
	EmailNotice    types.Bool   `tfsdk:"email_notice"`
	SmsNotice      types.Bool   `tfsdk:"sms_notice"`
	NoticeType     types.String `tfsdk:"notice_type"`
}

func (r *alidnsGtmInstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alidns_gtm_instance"
}

func (r *alidnsGtmInstanceResource) Schema(_ context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Alidns Gtm Instance resource.",
		Attributes: map[string]schema.Attribute{
			"instance_type": schema.StringAttribute{
				Description: "The type of Global Traffic Manager instance. Valid values: cn, intl.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("cn", "intl"),
				},
			},
			"instance_name": schema.StringAttribute{
				Description: "The name of Global Traffic Manager instance.",
				Required:    true,
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
			"package_edition": schema.StringAttribute{
				Description: "Paid package version. Valid values: ultimate, standard.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("standard", "ultimate"),
				},
			},
			// "health_check_task_count": schema.Int64Attribute{
			// 	Description: "The quota of detection tasks.",
			// 	Required:    true,
			// 	PlanModifiers: []planmodifier.Int64{
			// 		int64planmodifier.RequiresReplace(),
			// 	},
			// 	Validators: []validator.Int64{
			// 		int64validator.Between(0, 100000),
			// 	},
			// },
			"sms_notification_count": schema.Int64Attribute{
				Description: "The quota of SMS notifications.",
				Optional:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.Between(0, 100000),
				},
			},
			"id": schema.StringAttribute{
				Description: "The ID of Global Traffic Manager instance.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"renewal_status": schema.StringAttribute{
				Description: "Automatic renewal status.",
				Computed:    true,
			},
			"renew_period": schema.Int64Attribute{
				Description: "Automatic renewal period, the unit is month.",
				Computed:    true,
			},
			"public_cname_mode": schema.StringAttribute{
				Description: "The Public Network domain name access method. Valid " +
					"values: CUSTOM, SYSTEM_ASSIGN.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("SYSTEM_ASSIGN", "CUSTOM"),
				},
			},
			"public_rr": schema.StringAttribute{
				Description: "The CNAME access domain name.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"public_user_domain_name": schema.StringAttribute{
				Description: "The business domain name that the user uses on the Internet.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"public_zone_name": schema.StringAttribute{
				Description: "The domain name that is used to access GTM over the Internet.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ttl": schema.Int64Attribute{
				Description: "The global time to live. Valid values: 60, 120, 300, 600. Unit: second.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.OneOf(60, 120, 300, 600),
				},
			},
			"cname_type": schema.StringAttribute{
				Description: "The access type of the CNAME domain name. Valid value: PUBLIC.",
				Computed:    true,
			},
			"resource_group_id": schema.StringAttribute{
				Description: "The ID of the resource group.",
				Required:    true,
			},
			"alert_group": schema.ListAttribute{
				Description: "The alert group.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
			},
			"force_update": schema.BoolAttribute{
				Description: "The force update.",
				Optional:    true,
			},
			"strategy_mode": schema.StringAttribute{
				Description: "The type of the access policy. Valid values: GEO, LATENCY.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("GEO", "LATENCY"),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"alert_config": schema.SetNestedBlock{
				Description: "The alert notification methods. See the following Block alert_config.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"dingtalk_notice": schema.BoolAttribute{
							Description: "Whether to configure DingTalk notifications. Valid values: true, false.",
							Optional:    true,
							Computed:    true,
						},
						"email_notice": schema.BoolAttribute{
							Description: "Whether to configure mail notification. Valid values: true, false.",
							Optional:    true,
							Computed:    true,
						},
						"sms_notice": schema.BoolAttribute{
							Description: "Whether to configure SMS notification. Valid values: true, false.",
							Optional:    true,
							Computed:    true,
						},
						"notice_type": schema.StringAttribute{
							Description: "The Alarm Event Type. Valid values: ADDR_RESUME, " +
								"ADDR_ALERT, ADDR_POOL_GROUP_UNAVAILABLE, ADDR_POOL_GROUP_AVAILABLE " +
								"ACCESS_STRATEGY_POOL_GROUP_SWITCH, MONITOR_NODE_IP_CHANGE",
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf(getAlertConfigNoticeType()...),
							},
						},
					},
				},
			},
		},
	}
}

func (r *alidnsGtmInstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.baseClient = req.ProviderData.(alicloudClients).baseClient
	r.client = req.ProviderData.(alicloudClients).dnsClient
}

func (r *alidnsGtmInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state *alidnsGtmInstanceResourceModel
	planDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(planDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state = &alidnsGtmInstanceResourceModel{}

	//////////////////////// DATA VALIDATION ////////////////////////
	if plan.InstanceName.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("instance_name"),
			"Missing Instance Name",
			"GTM instance instance_name must not be empty.",
		)
		return
	}

	//////////////////////// CREATE INSTANCE ////////////////////////
	createInstanceRequest := &alicloudBaseClient.CreateInstanceRequest{
		RenewalStatus:    tea.String(plan.RenewalStatus.ValueString()),
		RenewPeriod:      tea.Int32(int32(plan.RenewPeriod.ValueInt64())),
		SubscriptionType: tea.String(plan.PaymentType.ValueString()),
		Period:           tea.Int32(1),
		ProductCode:      tea.String("dns"),
	}
	accountType := plan.InstanceType.ValueString()
	if accountType == "cn" {
		if plan.SmsNotificationCount.IsNull() || plan.SmsNotificationCount.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("sms_notification_count"),
				"sms_notification_count is required",
				"sms_notification_count is required when instance_type is cn.",
			)
			return
		}
		r.baseClient.Endpoint = tea.String("business.aliyuncs.com")
		createInstanceRequest.ProductType = tea.String("dns_gtm_public_cn")
		createInstanceRequest.Parameter = []*alicloudBaseClient.CreateInstanceRequestParameter{
			{
				Code:  tea.String("PackageEdition"),
				Value: tea.String(plan.PackageEdition.ValueString()),
			},
			{
				Code:  tea.String("HealthcheckTaskCount"),
				Value: tea.String(fmt.Sprint(0)),
			},
			{
				Code:  tea.String("SmsNotificationCount"),
				Value: tea.String(fmt.Sprint(plan.SmsNotificationCount.ValueInt64())),
			},
		}
	} else {
		r.baseClient.Endpoint = tea.String("business.ap-southeast-1.aliyuncs.com")
		createInstanceRequest.ProductType = tea.String("dns_gtm_public_intl")
		createInstanceRequest.Parameter = []*alicloudBaseClient.CreateInstanceRequestParameter{
			{
				Code:  tea.String("PackageEdition"),
				Value: tea.String(plan.PackageEdition.ValueString()),
			},
			{
				Code:  tea.String("HealthcheckTaskCount"),
				Value: tea.String(fmt.Sprint(0)),
			},
		}
	}

	createInstanceResponse := &alicloudBaseClient.CreateInstanceResponse{}
	var err error
	createGtmInstance := func() error {
		runtime := &util.RuntimeOptions{}
		createInstanceResponse, err = r.baseClient.CreateInstanceWithOptions(createInstanceRequest, runtime)
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
	err = backoff.Retry(createGtmInstance, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create GTM Instance",
			err.Error(),
		)
		return
	}

	/*
		There is a special case during development where the Body.Code is PAY.AMOUNT_LIMIT_EXCEEDED，
		but it's statusCode is 200. Therefore an additional error catching is configured.

		Example:
			map[body:map[Code:PAY.AMOUNT_LIMIT_EXCEEDED HostId:business.ap-southeast-1.aliyuncs.com Message:getUserDefaultPaymentMethod POC label fee is limit,havanaId:270584930149,result:[] requestId: 1F7A6DDB-51E7-30D0-A1A3-B98E6277B988
			Recommend:https://next.api.aliyun.com/troubleshoot?q=PAY.AMOUNT_LIMIT_EXCEEDED&product=BssOpenApi RequestId:1F7A6DDB-51E7-30D0-A1A3-B98E6277B988] headers:map[access-control-allow-origin:* connection:keep-alive
			content-length:380 content-type:application/json;charset=utf-8 date:Tue, 28 Feb 2023 07:26:00 GMT x-acs-request-id:1F7A6DDB-51E7-30D0-A1A3-B98E6277B988 x-acs-trace-id:73712e4c37d94c3720820d8e9de4ee60] statusCode:200]
	*/
	if *createInstanceResponse.Body.Code == "PAY.AMOUNT_LIMIT_EXCEEDED" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create GTM Instance",
			createInstanceResponse.String(),
		)
		return
	}

	instanceId := *createInstanceResponse.Body.Data.InstanceId
	state.RenewalStatus = plan.RenewalStatus
	state.RenewPeriod = plan.RenewPeriod
	state.Id = types.StringValue(instanceId)
	state.InstanceType = plan.InstanceType
	state.PaymentType = plan.PaymentType
	state.PackageEdition = plan.PackageEdition
	if accountType == "cn" {
		state.SmsNotificationCount = plan.SmsNotificationCount
	}
	state.AlertConfig = plan.AlertConfig
	state.AlertGroup = plan.AlertGroup

	createInstanceSetState := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(createInstanceSetState...)
	if resp.Diagnostics.HasError() {
		return
	}

	//////////////////////// UPDATE INSTANCE ////////////////////////
	/*
		Do not check resp.Diagnostics.HasError() so that Terraform will update its
		state as the update process include three API calls.
	*/
	updateInstanceDiags := r.updateGtmInstance(plan, state)
	resp.Diagnostics.Append(updateInstanceDiags...)
	updateInstanceSetState := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(updateInstanceSetState...)
	if resp.Diagnostics.HasError() {
		return
	}

	//////////////////////// READ INSTANCE ////////////////////////
	readInstancediags := r.readGtmInstance(state)
	resp.Diagnostics.Append(readInstancediags...)
	if resp.Diagnostics.HasError() {
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *alidnsGtmInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *alidnsGtmInstanceResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readInstanceDiags := r.readGtmInstance(state)
	resp.Diagnostics.Append(readInstanceDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.RenewalStatus.ValueString() != "AutoRenewal" || state.RenewPeriod.ValueInt64() != 1 {
		resp.Diagnostics.AddWarning(
			"Auto Renewal Status Changed Outside Terraform",
			"The renewal will be forced update to AutoRenewal and 1 month auto "+
				"renew period after applying changes update.",
		)
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *alidnsGtmInstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state *alidnsGtmInstanceResourceModel
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

	/*
		Do not check resp.Diagnostics.HasError() so that Terraform will update its
		state as the update process include three API calls.
	*/
	updateInstanceDiags := r.updateGtmInstance(plan, state)
	resp.Diagnostics.Append(updateInstanceDiags...)

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

}

func (r *alidnsGtmInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alidnsGtmInstanceResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	setRenewalRequest := &alicloudBaseClient.SetRenewalRequest{
		InstanceIDs:   tea.String(state.Id.ValueString()),
		RenewalStatus: tea.String("NotRenewal"),
		ProductCode:   tea.String("dns"),
	}

	var clientEndpoint string
	if state.InstanceType.ValueString() == "cn" {
		clientEndpoint = "business.aliyuncs.com"
	} else {
		clientEndpoint = "business.ap-southeast-1.aliyuncs.com"
	}
	err := r.setInstanceRenewal(clientEndpoint, setRenewalRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Set GTM Manual Renewal",
			fmt.Sprint(state),
		)
	}
}

func (r *alidnsGtmInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *alidnsGtmInstanceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If the entire plan is null, the resource is planned for destruction.
	if req.Plan.Raw.IsNull() {
		resp.Diagnostics.AddWarning(
			"Cannot destroy AliCloud GTM Instance",
			"Terraform will not destroy AliCloud GTM Instance as the instance is "+
				"subscription based. Instead, Terraform will set the renewal status "+
				"to NotRenewal so that the instance will being deleted on expiration date",
		)
	} else {
		var plan *alidnsGtmInstanceResourceModel
		getPlanDiags := req.Plan.Get(ctx, &plan)
		resp.Diagnostics.Append(getPlanDiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Assign some default value for computed value
		plan.CnameType = types.StringValue("PUBLIC")
		if len(plan.AlertConfig) > 0 {
			for _, alertConfig := range plan.AlertConfig {
				if alertConfig.DingtalkNotice.IsNull() || alertConfig.DingtalkNotice.IsUnknown() {
					alertConfig.DingtalkNotice = types.BoolValue(false)
				}
				if alertConfig.EmailNotice.IsNull() || alertConfig.EmailNotice.IsUnknown() {
					alertConfig.EmailNotice = types.BoolValue(false)
				}
				if alertConfig.SmsNotice.IsNull() || alertConfig.SmsNotice.IsUnknown() {
					alertConfig.SmsNotice = types.BoolValue(false)
				}
			}
		} else {
			plan.AlertConfig = []*alertConfig{}
			for _, noticeType := range getAlertConfigNoticeType() {
				plan.AlertConfig = append(
					plan.AlertConfig,
					&alertConfig{
						DingtalkNotice: types.BoolValue(false),
						EmailNotice:    types.BoolValue(false),
						SmsNotice:      types.BoolValue(false),
						NoticeType:     types.StringValue(noticeType),
					},
				)
			}
		}
		setAlertConfigDiags := resp.Plan.SetAttribute(ctx, path.Root("alert_config"), plan.AlertConfig)
		resp.Diagnostics.Append(setAlertConfigDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if plan.PublicCnameMode.IsNull() || plan.PublicCnameMode.IsUnknown() {
			plan.PublicCnameMode = types.StringValue("SYSTEM_ASSIGN")
		} else if plan.PublicCnameMode.ValueString() != "SYSTEM_ASSIGN" {
			if plan.PublicRr.IsNull() || plan.PublicRr.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					path.Root("public_rr"),
					"Missing Public Rr",
					"public_rr must be configured when public_cname_mode is 'CUSTOM'",
				)
			}
			if plan.PublicZoneName.IsNull() || plan.PublicRr.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					path.Root("public_zone_name"),
					"Missing Public Zone Name",
					"public_zone_name must be configured when public_cname_mode is 'CUSTOM'",
				)
			}
			if resp.Diagnostics.HasError() {
				return
			}
		}

		/*
			Force the plan attributes' value of the following:
				RenewalStatus = "AutoRenewal"
				RenewPeriod   = 1
			So that when detected changes outside Terraform, it will be auto
			reverted back to AutoRenewal on AliCloud.
		*/
		plan.RenewPeriod = types.Int64Value(1)
		plan.RenewalStatus = types.StringValue("AutoRenewal")

		resp.Plan.Set(ctx, &plan)
		if resp.Diagnostics.HasError() {
			return
		}
	}
}

func (r *alidnsGtmInstanceResource) readGtmInstance(state *alidnsGtmInstanceResourceModel) diag.Diagnostics {
	describeDnsGtmInstanceResponse := &alicloudDnsClient.DescribeDnsGtmInstanceResponse{}
	var err error
	createGtmInstance := func() error {
		describeGtmInstanceRequest := &alicloudDnsClient.DescribeDnsGtmInstanceRequest{
			InstanceId: tea.String(state.Id.ValueString()),
		}
		runtime := &util.RuntimeOptions{}
		describeDnsGtmInstanceResponse, err = r.client.DescribeDnsGtmInstanceWithOptions(describeGtmInstanceRequest, runtime)
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
	err = backoff.Retry(createGtmInstance, reconnectBackoff)
	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Failed to Find GTM Instance",
				err.Error(),
			),
		}
	}

	var accountType string
	if describeDnsGtmInstanceResponse.Body.UsedQuota.SmsUsedCount == nil {
		accountType = "intl"
		r.baseClient.Endpoint = tea.String("business.ap-southeast-1.aliyuncs.com")
	} else {
		accountType = "cn"
		r.baseClient.Endpoint = tea.String("business.aliyuncs.com")
	}

	queryAvailableInstancesRequest := &alicloudBaseClient.QueryAvailableInstancesRequest{
		InstanceIDs: tea.String(*describeDnsGtmInstanceResponse.Body.InstanceId),
	}
	queryAvailableInstancesResponse := &alicloudBaseClient.QueryAvailableInstancesResponse{}
	queryGtmInstance := func() error {
		runtime := &util.RuntimeOptions{}
		queryAvailableInstancesResponse, err = r.baseClient.QueryAvailableInstancesWithOptions(queryAvailableInstancesRequest, runtime)
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
	reconnectBackoff = backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(queryGtmInstance, reconnectBackoff)
	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Failed to Find GTM Instance",
				err.Error(),
			),
		}
	}

	var renewalStatus string
	var renewalDuration int32
	renewalStatus = *queryAvailableInstancesResponse.Body.Data.InstanceList[0].RenewStatus
	if renewalStatus == "AutoRenewal" {
		if *queryAvailableInstancesResponse.Body.Data.InstanceList[0].RenewalDurationUnit == "Y" {
			renewalDuration = *queryAvailableInstancesResponse.Body.Data.InstanceList[0].RenewalDuration * 12
		} else {
			renewalDuration = *queryAvailableInstancesResponse.Body.Data.InstanceList[0].RenewalDuration
		}
	} else {
		renewalDuration = 0
	}

	state.RenewPeriod = types.Int64Value(int64(renewalDuration))
	state.RenewalStatus = types.StringValue(renewalStatus)
	state.InstanceType = types.StringValue(accountType)
	state.Id = types.StringValue(*describeDnsGtmInstanceResponse.Body.InstanceId)
	state.ResourceGroupID = types.StringValue(*describeDnsGtmInstanceResponse.Body.ResourceGroupId)
	state.PaymentType = types.StringValue(*describeDnsGtmInstanceResponse.Body.PaymentType)
	state.PackageEdition = types.StringValue(*describeDnsGtmInstanceResponse.Body.VersionCode)
	if describeDnsGtmInstanceResponse.Body.Config.CnameType != nil {
		state.CnameType = types.StringValue(*describeDnsGtmInstanceResponse.Body.Config.CnameType)
	}
	if describeDnsGtmInstanceResponse.Body.Config.InstanceName != nil {
		state.InstanceName = types.StringValue(*describeDnsGtmInstanceResponse.Body.Config.InstanceName)
	}
	if describeDnsGtmInstanceResponse.Body.Config.StrategyMode != nil {
		state.StrategyMode = types.StringValue(*describeDnsGtmInstanceResponse.Body.Config.StrategyMode)
	}
	if describeDnsGtmInstanceResponse.Body.Config.PublicCnameMode != nil {
		state.PublicCnameMode = types.StringValue(*describeDnsGtmInstanceResponse.Body.Config.PublicCnameMode)
	}
	if describeDnsGtmInstanceResponse.Body.Config.PublicRr != nil {
		state.PublicRr = types.StringValue(*describeDnsGtmInstanceResponse.Body.Config.PublicRr)
	}
	if describeDnsGtmInstanceResponse.Body.Config.PublicUserDomainName != nil {
		state.PublicUserDomainName = types.StringValue(*describeDnsGtmInstanceResponse.Body.Config.PublicUserDomainName)
	}
	if describeDnsGtmInstanceResponse.Body.Config.PubicZoneName != nil {
		state.PublicZoneName = types.StringValue(*describeDnsGtmInstanceResponse.Body.Config.PubicZoneName)
	}
	if describeDnsGtmInstanceResponse.Body.Config.Ttl != nil {
		state.Ttl = types.Int64Value(int64(*describeDnsGtmInstanceResponse.Body.Config.Ttl))
	}
	if describeDnsGtmInstanceResponse.Body.Config.AlertGroup != nil {
		alertGroupList, err := convertJsonStringToListString(*describeDnsGtmInstanceResponse.Body.Config.AlertGroup)
		if err != nil {
			return diag.Diagnostics{
				diag.NewErrorDiagnostic(
					"[ERROR] Internal Code Error",
					err.Error(),
				),
			}
		}
		alertGroups := []attr.Value{}
		for _, x := range alertGroupList {
			alertGroups = append(alertGroups, types.StringValue(x))
		}
		state.AlertGroup = types.ListValueMust(types.StringType, alertGroups)
	}
	alertConfigsState := []*alertConfig{}
	if describeDnsGtmInstanceResponse.Body.Config.AlertConfig != nil {
		alertConfigsList := *describeDnsGtmInstanceResponse.Body.Config.AlertConfig
		for _, x := range alertConfigsList.AlertConfig {
			alertConfig := alertConfig{
				DingtalkNotice: types.BoolValue(*x.DingtalkNotice),
				EmailNotice:    types.BoolValue(*x.EmailNotice),
				SmsNotice:      types.BoolValue(*x.SmsNotice),
				NoticeType:     types.StringValue(*x.NoticeType),
			}
			alertConfigsState = append(alertConfigsState, &alertConfig)
		}
	}
	state.AlertConfig = alertConfigsState
	return nil
}

func (r *alidnsGtmInstanceResource) updateGtmInstance(plan *alidnsGtmInstanceResourceModel, state *alidnsGtmInstanceResourceModel) diag.Diagnostics {
	var err error

	// SetRenewal
	if state.RenewalStatus.ValueString() != "AutoRenewal" || state.RenewPeriod.ValueInt64() != 1 {
		var clientEndpoint string
		if state.InstanceType.ValueString() == "cn" {
			clientEndpoint = "business.aliyuncs.com"
		} else {
			clientEndpoint = "business.ap-southeast-1.aliyuncs.com"
		}

		setRenewalRequest := &alicloudBaseClient.SetRenewalRequest{
			InstanceIDs:       tea.String(state.Id.ValueString()),
			RenewalStatus:     tea.String("AutoRenewal"),
			RenewalPeriod:     tea.Int32(1),
			RenewalPeriodUnit: tea.String("M"),
			ProductCode:       tea.String("dns"),
		}

		err = r.setInstanceRenewal(clientEndpoint, setRenewalRequest)
		if err != nil {
			return diag.Diagnostics{
				diag.NewErrorDiagnostic(
					"[API ERROR] Failed to Set GTM Auto Renewal",
					err.Error(),
				),
			}
		}
		state.RenewalStatus = types.StringValue("AutoRenewal")
		state.RenewPeriod = types.Int64Value(1)
	}

	// MoveGtmResourceGroupWithOptions
	if !(plan.ResourceGroupID.IsNull() || plan.ResourceGroupID.IsUnknown()) {
		moveGtmResourceGroupRequest := &alicloudDnsClient.MoveGtmResourceGroupRequest{
			ResourceId:         tea.String(state.Id.ValueString()),
			NewResourceGroupId: tea.String(plan.ResourceGroupID.ValueString()),
		}
		moveGtmInstance := func() error {
			runtime := &util.RuntimeOptions{}
			_, err := r.client.MoveGtmResourceGroupWithOptions(moveGtmResourceGroupRequest, runtime)
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
		err = backoff.Retry(moveGtmInstance, reconnectBackoff)
		if err != nil {
			return diag.Diagnostics{
				diag.NewErrorDiagnostic(
					"[API ERROR] Failed to Move GTM Resource Group",
					err.Error(),
				),
			}
		}

		state.ResourceGroupID = plan.ResourceGroupID
	}

	// SwitchDnsGtmInstanceStrategyModeWithOptions
	if !(plan.StrategyMode.IsNull() || plan.StrategyMode.IsUnknown()) {
		switchDnsGtmInstanceStrategyModeRequest := &alicloudDnsClient.SwitchDnsGtmInstanceStrategyModeRequest{
			InstanceId:   tea.String(state.Id.ValueString()),
			StrategyMode: tea.String(plan.StrategyMode.ValueString()),
		}
		createGtmInstance := func() error {
			runtime := &util.RuntimeOptions{}
			_, err = r.client.SwitchDnsGtmInstanceStrategyModeWithOptions(switchDnsGtmInstanceStrategyModeRequest, runtime)
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
		err = backoff.Retry(createGtmInstance, reconnectBackoff)
		if err != nil {
			return diag.Diagnostics{
				diag.NewErrorDiagnostic(
					"[API ERROR] Failed to Switch Strategy Mode",
					err.Error(),
				),
			}
		}
		state.StrategyMode = plan.StrategyMode
	}

	// UpdateDnsGtmInstanceGlobalConfigWithOptions
	UpdateInstanceRequest := &alicloudDnsClient.UpdateDnsGtmInstanceGlobalConfigRequest{}
	UpdateInstanceRequest.InstanceId = tea.String(state.Id.ValueString())
	for _, planAlertConfig := range plan.AlertConfig {
		alertConfigRequest := &alicloudDnsClient.UpdateDnsGtmInstanceGlobalConfigRequestAlertConfig{}
		alertConfigRequest.DingtalkNotice = tea.Bool(planAlertConfig.DingtalkNotice.ValueBool())
		alertConfigRequest.EmailNotice = tea.Bool(planAlertConfig.EmailNotice.ValueBool())
		alertConfigRequest.SmsNotice = tea.Bool(planAlertConfig.SmsNotice.ValueBool())
		alertConfigRequest.NoticeType = tea.String(planAlertConfig.NoticeType.ValueString())
		UpdateInstanceRequest.AlertConfig = append(UpdateInstanceRequest.AlertConfig, alertConfigRequest)
	}
	state.AlertConfig = plan.AlertConfig

	var planAlertGroupList []string
	for _, x := range plan.AlertGroup.Elements() {
		planAlertGroupList = append(planAlertGroupList, trimStringQuotes(x.String()))
	}
	UpdateInstanceRequest.AlertGroup = tea.String(convertListStringToJsonString(planAlertGroupList))
	state.AlertGroup = plan.AlertGroup

	UpdateInstanceRequest.InstanceName = tea.String(plan.InstanceName.ValueString())
	state.InstanceName = plan.InstanceName

	UpdateInstanceRequest.Ttl = tea.Int32(int32(plan.Ttl.ValueInt64()))
	state.Ttl = plan.Ttl

	UpdateInstanceRequest.CnameType = tea.String(plan.CnameType.ValueString())
	state.CnameType = plan.CnameType

	UpdateInstanceRequest.PublicCnameMode = tea.String(plan.PublicCnameMode.ValueString())
	state.PublicCnameMode = plan.PublicCnameMode

	if !(plan.PublicUserDomainName.IsNull() || plan.PublicUserDomainName.IsUnknown()) {
		UpdateInstanceRequest.PublicUserDomainName = tea.String(plan.PublicUserDomainName.ValueString())
		state.PublicUserDomainName = plan.PublicUserDomainName
	} else {
		UpdateInstanceRequest.PublicUserDomainName = tea.String(fmt.Sprintf("%s.com", state.Id.ValueString()))
	}

	if plan.CnameType.ValueString() != "SYSTEM_ASSIGN" && !(plan.PublicRr.IsNull() || plan.PublicRr.IsUnknown()) {
		UpdateInstanceRequest.PublicRr = tea.String(plan.PublicRr.ValueString())
		state.PublicRr = plan.PublicRr
	}

	if plan.CnameType.ValueString() != "SYSTEM_ASSIGN" && !(plan.PublicZoneName.IsNull() || plan.PublicZoneName.IsUnknown()) {
		UpdateInstanceRequest.PublicZoneName = tea.String(plan.PublicZoneName.ValueString())
		state.PublicZoneName = plan.PublicZoneName
	}

	createGtmInstance := func() error {
		runtime := &util.RuntimeOptions{}
		_, err = r.client.UpdateDnsGtmInstanceGlobalConfigWithOptions(UpdateInstanceRequest, runtime)
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
	err = backoff.Retry(createGtmInstance, reconnectBackoff)
	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Failed to Update DNS Gtm Instance",
				err.Error(),
			),
		}
	}

	return nil
}

func (r alidnsGtmInstanceResource) setInstanceRenewal(clientEndpoint string, req *alicloudBaseClient.SetRenewalRequest) error {
	r.baseClient.Endpoint = tea.String(clientEndpoint)

	setRenewal := func() error {
		runtime := &util.RuntimeOptions{}
		_, err := r.baseClient.SetRenewalWithOptions(req, runtime)
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
	return backoff.Retry(setRenewal, reconnectBackoff)
}
