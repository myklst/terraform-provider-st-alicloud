package alicloud

import (
	"context"
	"time"

	alicloudCmsClient "github.com/alibabacloud-go/cms-20190101/v8/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &cmsSystemEventGroupBindingResource{}
	_ resource.ResourceWithConfigure = &cmsSystemEventGroupBindingResource{}
)

func NewCmsSystemEventGroupBindingResource() resource.Resource {
	return &cmsSystemEventGroupBindingResource{}
}

type cmsSystemEventGroupBindingResource struct {
	client *alicloudCmsClient.Client
}

type cmsSystemEventGroupBindingResourceModel struct {
	RuleName         types.String `tfsdk:"rule_name"`
	ContactGroupName types.String `tfsdk:"contact_group_name"`
	Level            types.String `tfsdk:"level"`
}

func (r *cmsSystemEventGroupBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cms_system_event_group_binding"
}

func (r *cmsSystemEventGroupBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Alicloud CMS System Event Group Binding Resource.",
		Attributes: map[string]schema.Attribute{
			"rule_name": schema.StringAttribute{
				Description: "The name of the event-triggered alert rule.",
				Required:    true,
			},
			"contact_group_name": schema.StringAttribute{
				Description: "The name of the alert contact group.",
				Required:    true,
			},
			"level": schema.StringAttribute{
				Description: "The alert notification methods.",
				Required:    true,
			},
		},
	}
}

func (r *cmsSystemEventGroupBindingResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).cmsClient
}

func (r *cmsSystemEventGroupBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *cmsSystemEventGroupBindingResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.bindSystemEventGroup(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Bind System Event Group.",
			err.Error(),
		)
		return
	}

	state := &cmsSystemEventGroupBindingResourceModel{}
	state.RuleName = plan.RuleName
	state.ContactGroupName = plan.ContactGroupName
	state.Level = plan.Level

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cmsSystemEventGroupBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *cmsSystemEventGroupBindingResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readSystemEventGroup := func() error {
		runtime := &util.RuntimeOptions{}

		readSystemEventGroupRequest := &alicloudCmsClient.DescribeEventRuleTargetListRequest{
			RuleName: tea.String(state.RuleName.ValueString()),
		}

		readSystemEventGroupResponse, err := r.client.DescribeEventRuleTargetListWithOptions(readSystemEventGroupRequest, runtime)
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

		if readSystemEventGroupResponse.Body.ContactParameters != nil {
			for _, contactGroup := range readSystemEventGroupResponse.Body.ContactParameters.ContactParameter {
				state.ContactGroupName = types.StringValue(*contactGroup.ContactGroupName)
				state.Level = types.StringValue(*contactGroup.Level)
			}

			setStateDiags := resp.State.Set(ctx, &state)
			resp.Diagnostics.Append(setStateDiags...)
			if resp.Diagnostics.HasError() {
				resp.Diagnostics.AddError(
					"[API ERROR] Failed to Set Read CMS System Event Group to State",
					err.Error(),
				)
			}
		} else {
			resp.State.RemoveResource(ctx)
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(readSystemEventGroup, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Read Users for Group",
			err.Error(),
		)
		return
	}
}

func (r *cmsSystemEventGroupBindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *cmsSystemEventGroupBindingResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.bindSystemEventGroup(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Bind System Event Group.",
			err.Error(),
		)
		return
	}

	state := &cmsSystemEventGroupBindingResourceModel{}
	state.RuleName = plan.RuleName
	state.ContactGroupName = plan.ContactGroupName
	state.Level = plan.Level

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cmsSystemEventGroupBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {

}

func (r *cmsSystemEventGroupBindingResource) bindSystemEventGroup(plan *cmsSystemEventGroupBindingResourceModel) (err error) {
	contactParameters := &alicloudCmsClient.PutEventRuleTargetsRequestContactParameters{
		ContactGroupName: tea.String(plan.ContactGroupName.ValueString()),
		Level:            tea.String(plan.Level.ValueString()),
	}

	bindSystemEventGroupRequest := &alicloudCmsClient.PutEventRuleTargetsRequest{
		RuleName:          tea.String(plan.RuleName.ValueString()),
		ContactParameters: []*alicloudCmsClient.PutEventRuleTargetsRequestContactParameters{contactParameters},
	}

	bindSystemEventGroup := func() error {
		runtime := &util.RuntimeOptions{}

		if _, err := r.client.PutEventRuleTargetsWithOptions(bindSystemEventGroupRequest, runtime); err != nil {
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
	return backoff.Retry(bindSystemEventGroup, reconnectBackoff)
}
