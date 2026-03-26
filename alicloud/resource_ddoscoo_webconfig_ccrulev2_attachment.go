package alicloud

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	alicloudAntiddosClient "github.com/alibabacloud-go/ddoscoo-20200101/v4/client"
	openapiutil "github.com/alibabacloud-go/openapi-util/service"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/itchyny/gojq"
)

var (
	_ resource.Resource                = &ddoscooWebconfigCCRuleV2Resource{}
	_ resource.ResourceWithConfigure   = &ddoscooWebconfigCCRuleV2Resource{}
	_ resource.ResourceWithImportState = &ddoscooWebconfigCCRuleV2Resource{}
)

func NewDdosCooWebconfigCCRuleV2Resource() resource.Resource {
	return &ddoscooWebconfigCCRuleV2Resource{}
}

type ddoscooWebconfigCCRuleV2Resource struct {
	client *alicloudAntiddosClient.Client
}

type statusCodeModel struct {
	Enabled        types.Bool  `tfsdk:"enabled"`
	Code           types.Int64 `tfsdk:"code"`
	UseRatio       types.Bool  `tfsdk:"use_ratio"`
	RatioThreshold types.Int64 `tfsdk:"ratio_threshold"`
	CountThreshold types.Int64 `tfsdk:"count_threshold"`
}

type statisticsModel struct {
	Mode       types.String `tfsdk:"mode"`
	Field      types.String `tfsdk:"field"`
	HeaderName types.String `tfsdk:"header_name"`
}

type rateLimitModel struct {
	Interval  types.Int64  `tfsdk:"interval"`
	TTL       types.Int64  `tfsdk:"ttl"`
	Threshold types.Int64  `tfsdk:"threshold"`
	Subkey    types.String `tfsdk:"subkey"`
	Target    types.String `tfsdk:"target"`
}

type conditionModel struct {
	Field       types.String `tfsdk:"field"`
	MatchMethod types.String `tfsdk:"match_method"`
	HeaderName  types.String `tfsdk:"header_name"`
	Content     types.String `tfsdk:"content"`
	ContentList types.List   `tfsdk:"content_list"`
}

type ruleModel struct {
	Action     types.String     `tfsdk:"action"`
	Name       types.String     `tfsdk:"name"`
	Condition  []conditionModel `tfsdk:"condition"`
	RateLimit  *rateLimitModel  `tfsdk:"rate_limit"`
	Statistics *statisticsModel `tfsdk:"statistics"`
	StatusCode *statusCodeModel `tfsdk:"status_code"`
}

type ddoscooWebconfigCCRuleV2Model struct {
	Domain   types.String `tfsdk:"domain"`
	Expires  types.Int64  `tfsdk:"expires"`
	RuleList []*ruleModel `tfsdk:"rule_list"`
}

func (r *ddoscooWebconfigCCRuleV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ddoscoo_webconfig_ccrule_v2"
}

func (r *ddoscooWebconfigCCRuleV2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Create v2 CCRules for this domain",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Description: "Domain name.",
				Required:    true,
			},
			"expires": schema.Int64Attribute{
				Description: "The validity period of the rule. Unit: seconds. The value 0 indicates that the rule is permanently valid",
				Required:    false,
				Optional:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"rule_list": &schema.SetNestedBlock{
				Description: "List of CC rules v2 to be added.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"action": &schema.StringAttribute{
							Description: "The action to perform upon matching. Valid values: block, challenge, watch.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("block", "challenge", "watch"),
							},
						},
						"name": &schema.StringAttribute{
							Description: "Name of the rule.",
							Required:    true,
						},
						"condition": &schema.ListNestedAttribute{
							Required: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"field": &schema.StringAttribute{
										Description: "Matching field.",
										Required:    true,
									},
									"match_method": &schema.StringAttribute{
										Description: "Matching method.",
										Required:    true,
									},
									"header_name": &schema.StringAttribute{
										Description: "Custom HTTP header field name.",
										Required:    false,
										Optional:    true,
									},
									"content": &schema.StringAttribute{
										Description: "Matching content.",
										Required:    false,
										Optional:    true,
										Validators: []validator.String{
											stringvalidator.ConflictsWith(
												path.MatchRelative().
													AtParent().
													AtName("content_list"),
											),
										},
									},
									"content_list": &schema.ListAttribute{
										ElementType: types.StringType,
										Description: "Contents if the matching is one of multiple.",
										Required:    false,
										Optional:    true,
										Validators: []validator.List{
											listvalidator.ConflictsWith(
												path.MatchRelative().
													AtParent().
													AtName("content"),
											),
										},
									},
								},
							},
						},
						"rate_limit": &schema.SingleNestedAttribute{
							Description: "The frequency statistics.",
							Required:    false,
							Optional:    true,
							Attributes: map[string]schema.Attribute{
								"interval": schema.Int64Attribute{
									Description: "The statistics duration.",
									Required:    true,
								},
								"ttl": schema.Int64Attribute{
									Description: "Action duration.",
									Required:    true,
								},
								"threshold": schema.Int64Attribute{
									Description: "Threshold",
									Required:    true,
								},
								"subkey": schema.StringAttribute{
									Description: "Field name (set only when the statistics source is header).",
									Required:    false,
									Optional:    true,
								},
								"target": schema.StringAttribute{
									Description: "Statistics source, supports ip and header",
									Required:    true,
									Validators: []validator.String{
										stringvalidator.OneOf("ip", "header"),
									},
								},
							},
						},
						"statistics": &schema.SingleNestedAttribute{
							Description: "The statistics after deduplication. By default, the system collects statistics before deduplication.",
							Required:    false,
							Optional:    true,
							Attributes: map[string]schema.Attribute{
								"mode": schema.StringAttribute{
									Description: "Indicates whether the system collects statistics after deduplication. Valid values: count, distinct",
									Required:    true,
									Validators: []validator.String{
										stringvalidator.OneOf("count", "distinct"),
									},
								},
								"field": schema.StringAttribute{
									Description: "The statistical method.",
									Required:    true,
								},
								"header_name": schema.StringAttribute{
									Description: "The name of the header. This parameter is required only when the Field parameter is set to header.",
									Required:    false,
									Optional:    true,
								},
							},
						},
						"status_code": &schema.SingleNestedAttribute{
							Description: "The status codes",
							Required:    false,
							Optional:    true,
							Attributes: map[string]schema.Attribute{
								"enabled": schema.BoolAttribute{
									Description: "Indicates whether the status code is enabled.",
									Required:    true,
								},
								"code": schema.Int64Attribute{
									Description: "The status code.",
									Required:    true,
								},
								"use_ratio": schema.BoolAttribute{
									Description: "Indicates whether to use a ratio.",
									Required:    true,
								},
								"ratio_threshold": schema.Int64Attribute{
									Description: "If a ratio is used, the handling action is triggered only when the number of requests of the corresponding status code reaches the value of ratio_threshold",
									Required:    false,
									Optional:    true,
								},
								"count_threshold": schema.Int64Attribute{
									Description: "If a ratio is not used, the handling action is triggered only when the number of requests of the corresponding status code reaches the value of count_threshold",
									Required:    true,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *ddoscooWebconfigCCRuleV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).antiddosClient
}

func (r *ddoscooWebconfigCCRuleV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *ddoscooWebconfigCCRuleV2Model
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.createCCRuleV2(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to create CC Rule V2.",
			err.Error(),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ddoscooWebconfigCCRuleV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *ddoscooWebconfigCCRuleV2Model
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readResp, err := r.describeCCRuleV2(state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read CC Rule V2.",
			err.Error(),
		)
		return
	}

	rulesList := []*ruleModel{}
	for _, rule := range readResp.WebCCRules {
		rulesList = append(rulesList, &ruleModel{
			Action: types.StringPointerValue(rule.RuleDetail.Action),
			Name:   types.StringPointerValue(rule.Name),
			Condition: func() []conditionModel {
				conditions := []conditionModel{}

				for _, cond := range rule.RuleDetail.Condition {
					conditions = append(conditions, conditionModel{
						Field:       types.StringPointerValue(cond.Field),
						MatchMethod: types.StringPointerValue(cond.MatchMethod),
						HeaderName: (func() basetypes.StringValue {
							if cond.HeaderName == nil {
								return types.StringNull()
							}
							if *cond.HeaderName == "" {
								return types.StringNull()
							}
							return types.StringPointerValue(cond.HeaderName)
						})(),
						Content: types.StringPointerValue(cond.Content),
						ContentList: (func() basetypes.ListValue {
							if cond.ContentList == nil {
								return types.ListNull(types.StringType)
							}

							contents := []string{}
							_ = json.Unmarshal([]byte(*cond.ContentList), &contents)

							contentList := []attr.Value{}
							for _, i := range contents {
								contentList = append(contentList, types.StringValue(i))
							}

							return types.ListValueMust(types.StringType, contentList)
						})(),
					})
				}

				return conditions
			}(),
			RateLimit: (func() *rateLimitModel {
				if *rule.RuleDetail.RateLimit != (alicloudAntiddosClient.DescribeWebCCRulesV2ResponseBodyWebCCRulesRuleDetailRateLimit{}) {
					return &rateLimitModel{
						Interval:  types.Int64Value(int64(*rule.RuleDetail.RateLimit.Interval)),
						TTL:       types.Int64Value(int64(*rule.RuleDetail.RateLimit.Ttl)),
						Threshold: types.Int64Value(int64(*rule.RuleDetail.RateLimit.Threshold)),
						Subkey: (func() basetypes.StringValue {
							if rule.RuleDetail.RateLimit.SubKey == nil {
								return types.StringNull()
							}
							if *rule.RuleDetail.RateLimit.SubKey == "" {
								return types.StringNull()
							}
							return types.StringPointerValue(rule.RuleDetail.RateLimit.SubKey)
						})(),
						Target: types.StringPointerValue(rule.RuleDetail.RateLimit.Target),
					}
				} else {
					return nil
				}
			})(),
			StatusCode: (func() *statusCodeModel {
				if *rule.RuleDetail.StatusCode != (alicloudAntiddosClient.DescribeWebCCRulesV2ResponseBodyWebCCRulesRuleDetailStatusCode{}) {
					return &statusCodeModel{
						Enabled: types.BoolPointerValue(rule.RuleDetail.StatusCode.Enabled),
						Code: types.Int64PointerValue(func() *int64 {
							if rule.RuleDetail.StatusCode.Code != nil {
								ratioThreshold := int64(int32(*rule.RuleDetail.StatusCode.Code))
								return &ratioThreshold
							} else {
								return nil
							}
						}()),
						UseRatio: types.BoolPointerValue(rule.RuleDetail.StatusCode.UseRatio),
						RatioThreshold: types.Int64PointerValue(func() *int64 {
							if rule.RuleDetail.StatusCode.RatioThreshold != nil {
								ratioThreshold := int64(int32(*rule.RuleDetail.StatusCode.RatioThreshold))
								return &ratioThreshold
							} else {
								return nil
							}
						}()),
						CountThreshold: types.Int64PointerValue(func() *int64 {
							if rule.RuleDetail.StatusCode.CountThreshold != nil {
								ratioThreshold := int64(int32(*rule.RuleDetail.StatusCode.CountThreshold))
								return &ratioThreshold
							} else {
								return nil
							}
						}()),
					}
				} else {
					return nil
				}
			})(),
			Statistics: (func() *statisticsModel {
				if *rule.RuleDetail.Statistics != (alicloudAntiddosClient.DescribeWebCCRulesV2ResponseBodyWebCCRulesRuleDetailStatistics{}) {
					return &statisticsModel{
						Mode:  types.StringPointerValue(rule.RuleDetail.Statistics.Mode),
						Field: types.StringPointerValue(rule.RuleDetail.Statistics.Field),
						HeaderName: (func() basetypes.StringValue {
							if rule.RuleDetail.Statistics.HeaderName == nil {
								return types.StringNull()
							}
							if *rule.RuleDetail.Statistics.HeaderName == "" {
								return types.StringNull()
							}
							return types.StringPointerValue(rule.RuleDetail.Statistics.HeaderName)
						})(),
					}
				} else {
					return nil
				}
			})(),
		})
	}
	state.RuleList = rulesList

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *ddoscooWebconfigCCRuleV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state *ddoscooWebconfigCCRuleV2Model
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rulesInPlan := map[string]*ruleModel{}
	rulesInState := map[string]*ruleModel{}

	for _, rule := range plan.RuleList {
		rulesInPlan[rule.Name.ValueString()] = rule
	}
	for _, rule := range state.RuleList {
		rulesInState[rule.Name.ValueString()] = rule
	}

	plannedRules := mapset.NewSetFromMapKeys(rulesInPlan)
	stateRules := mapset.NewSetFromMapKeys(rulesInState)

	updates := ddoscooWebconfigCCRuleV2Model{
		Domain:  plan.Domain,
		Expires: plan.Expires,
	}

	// New rules that are added in the Terraform update phase, after Terraform create
	newlyAddedRules := plannedRules.Difference(stateRules)
	for _, ruleName := range newlyAddedRules.ToSlice() {
		updates.RuleList = append(updates.RuleList, rulesInPlan[ruleName])
	}

	// Rules that are present in both state and plan. Some rules may have changed.
	// Further logic is needed to identify changed rules
	potentiallyChangedRules := plannedRules.Intersect(stateRules)
	for _, ruleName := range potentiallyChangedRules.ToSlice() {
		if !reflect.DeepEqual(rulesInPlan[ruleName], rulesInState[ruleName]) {
			updates.RuleList = append(updates.RuleList, rulesInPlan[ruleName])
		}
	}

	err := r.createCCRuleV2(&updates)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to update CC Rule V2.",
			err.Error(),
		)
		return
	}

	deletedRules := stateRules.Difference(plannedRules)
	if deletedRules.Cardinality() > 0 {
		err = r.deleteCCRuleV2(&ddoscooWebconfigCCRuleV2Model{
			Domain:  state.Domain,
			Expires: state.Expires,
			RuleList: (func() []*ruleModel {
				rules := []*ruleModel{}

				for _, r := range deletedRules.ToSlice() {
					rules = append(rules, &ruleModel{
						Name: types.StringValue(r),
					})
				}

				return rules
			})(),
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to update CC Rule V2, during the delete phase.",
				err.Error(),
			)
			return
		}
	}

	setStateDiags := resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ddoscooWebconfigCCRuleV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *ddoscooWebconfigCCRuleV2Model
	getPlanDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.deleteCCRuleV2(state)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to delete CC Rule V2.",
			err.Error(),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *ddoscooWebconfigCCRuleV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}

func (r *ddoscooWebconfigCCRuleV2Resource) createCCRuleV2(rule *ddoscooWebconfigCCRuleV2Model) error {
	type rateLimitModel struct {
		Interval  int     `json:"interval"`
		TTL       int     `json:"ttl"`
		Threshold int     `json:"threshold"`
		Subkey    *string `json:"subkey,omitempty"`
		Target    string  `json:"target"`
	}

	type conditionModel struct {
		Field       string   `json:"field"`
		MatchMethod string   `json:"match_method"`
		HeaderName  *string  `json:"header_name,omitempty"`
		Content     *string  `json:"content,omitempty"`
		ContentList []string `json:"content_list,omitempty"`
	}

	type statusCodeModel struct {
		Enable         bool   `json:"enable"`
		Code           int    `json:"code"`
		UseRatio       bool   `json:"use_ratio"`
		RatioThreshold *int64 `json:"ratio_threshold,omitempty"`
		CountThreshold *int64 `json:"count_threshold,omitempty"`
	}

	type statisticsModel struct {
		Mode       string  `json:"mode"`
		Field      string  `json:"field"`
		HeaderName *string `json:"header_name,omitempty"`
	}

	type ruleModel struct {
		Action     string           `json:"action"`
		Name       string           `json:"name"`
		Condition  []conditionModel `json:"condition"`
		RateLimit  *rateLimitModel  `json:"ratelimit,omitempty"`
		StatusCode *statusCodeModel `json:"status_code,omitempty"`
		Statistics *statisticsModel `json:"statistics,omitempty"`
	}

	rules := []ruleModel{}
	for _, ruleCfg := range rule.RuleList {
		rules = append(rules, ruleModel{
			Action: ruleCfg.Action.ValueString(),
			Name:   ruleCfg.Name.ValueString(),
			Condition: (func() []conditionModel {
				conditions := []conditionModel{}

				for _, cond := range ruleCfg.Condition {
					conditions = append(conditions, conditionModel{
						Field:       cond.Field.ValueString(),
						MatchMethod: cond.MatchMethod.ValueString(),
						HeaderName:  cond.HeaderName.ValueStringPointer(),
						Content:     cond.Content.ValueStringPointer(),
						ContentList: (func() []string {
							if len(cond.ContentList.Elements()) == 0 {
								return nil
							} else {
								contentList := []string{}
								cond.ContentList.ElementsAs(context.Background(), &contentList, true)
								return contentList
							}
						})(),
					})
				}

				return conditions
			})(),
			RateLimit: (func() *rateLimitModel {
				if ruleCfg.RateLimit != nil {
					return &rateLimitModel{
						Interval:  int(ruleCfg.RateLimit.Interval.ValueInt64()),
						TTL:       int(ruleCfg.RateLimit.TTL.ValueInt64()),
						Threshold: int(ruleCfg.RateLimit.Threshold.ValueInt64()),
						Subkey:    ruleCfg.RateLimit.Subkey.ValueStringPointer(),
						Target:    ruleCfg.RateLimit.Target.ValueString(),
					}
				} else {
					return nil
				}
			})(),
			StatusCode: (func() *statusCodeModel {
				if ruleCfg.StatusCode != nil {
					return &statusCodeModel{
						Enable:         ruleCfg.StatusCode.Enabled.ValueBool(),
						Code:           int(ruleCfg.StatusCode.Code.ValueInt64()),
						UseRatio:       ruleCfg.StatusCode.UseRatio.ValueBool(),
						RatioThreshold: ruleCfg.StatusCode.RatioThreshold.ValueInt64Pointer(),
						CountThreshold: ruleCfg.StatusCode.CountThreshold.ValueInt64Pointer(),
					}
				} else {
					return nil
				}
			})(),
			Statistics: (func() *statisticsModel {
				if ruleCfg.Statistics != nil {
					return &statisticsModel{
						Mode:       ruleCfg.Statistics.Mode.ValueString(),
						Field:      ruleCfg.Statistics.Field.ValueString(),
						HeaderName: ruleCfg.Statistics.HeaderName.ValueStringPointer(),
					}
				} else {
					return nil
				}
			})(),
		})
	}

	ruleListString, err := json.Marshal(rules)
	if err != nil {
		return err
	}

	ccRuleV2CreateReq := &alicloudAntiddosClient.ConfigWebCCRuleV2Request{}
	ccRuleV2CreateReq.SetDomain(rule.Domain.ValueString())
	ccRuleV2CreateReq.SetExpires(rule.Expires.ValueInt64())
	ccRuleV2CreateReq.SetRuleList(string(ruleListString))

	runtime := &util.RuntimeOptions{}
	_, _err := r.client.ConfigWebCCRuleV2WithOptions(ccRuleV2CreateReq, runtime)
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

func (r *ddoscooWebconfigCCRuleV2Resource) describeCCRuleV2(domain string) (*alicloudAntiddosClient.DescribeWebCCRulesV2ResponseBody, error) {
	runtime := &util.RuntimeOptions{}

	params := &openapi.Params{
		Action:      tea.String("DescribeWebCCRulesV2"),
		Version:     tea.String("2020-01-01"),
		Protocol:    tea.String("HTTPS"),
		Method:      tea.String("POST"),
		AuthType:    tea.String("AK"),
		Style:       tea.String("RPC"),
		Pathname:    tea.String("/"),
		ReqBodyType: tea.String("json"),
		BodyType:    tea.String("json"),
	}

	queries := map[string]any{}
	queries["Domain"] = tea.String(domain)

	request := &openapi.OpenApiRequest{
		Query: openapiutil.Query(queries),
	}

	_resp, _err := r.client.CallApi(params, request, runtime)
	if _err != nil {
		return nil, _err
	}

	// Within each CC Rule element, if ContentList is present, convert it to json string
	// This temporry gojq implementation is needed due to conversion issues in Aliyun Go SDK
	// Will be removed in once the Aiyun Go SDK errors are resolved.
	query, err := gojq.Parse(`
		.WebCCRules |= [ .[] | walk(
		  if type == "object"
		     and has("ContentList")
		     and (.ContentList | type == "array")
		     and all(.ContentList[]; type == "string")
		  then
		     .ContentList |= tostring
		  else
		     .
		  end
		) ] | .TotalCount |= tostring
	`)
	if err != nil {
		parseErr := err.(*gojq.ParseError)
		return nil, fmt.Errorf("cannot parse jq filter: %s, error on token: %s", parseErr.Error(), parseErr.Token)
	}

	output := []any{}
	iter := query.Run(_resp["body"])
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			if err, ok := err.(*gojq.HaltError); ok && err.Value() == nil {
				return nil, fmt.Errorf("jq halted")
			}
			if err != nil {
				return nil, fmt.Errorf("jq run error: %s", err.Error())
			}
		}
		output = append(output, v)
	}

	filteredResp := output[0]
	respBytes, err := json.Marshal(filteredResp)
	if err != nil {
		return nil, err
	}

	resp := alicloudAntiddosClient.DescribeWebCCRulesV2ResponseBody{}
	err = json.Unmarshal(respBytes, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (r *ddoscooWebconfigCCRuleV2Resource) deleteCCRuleV2(rule *ddoscooWebconfigCCRuleV2Model) error {
	ruleNames, err := (func() (*string, error) {
		ruleNames := []string{}

		for _, entry := range rule.RuleList {
			ruleNames = append(ruleNames, entry.Name.ValueString())
		}

		str, err := json.Marshal(ruleNames)
		if err != nil {
			return nil, err
		}

		return tea.String(string(str)), nil
	})()
	if err != nil {
		return err
	}

	ccRuleV2DeleteReq := &alicloudAntiddosClient.DeleteWebCCRuleV2Request{
		Domain:    rule.Domain.ValueStringPointer(),
		RuleNames: ruleNames,
	}

	runtime := &util.RuntimeOptions{}
	_, _err := r.client.DeleteWebCCRuleV2WithOptions(ccRuleV2DeleteReq, runtime)
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
