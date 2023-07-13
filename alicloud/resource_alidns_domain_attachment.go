package alicloud

import (
	"context"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudDnsClient "github.com/alibabacloud-go/alidns-20150109/v4/client"
	"github.com/alibabacloud-go/tea/tea"
)

var (
	_ resource.Resource              = &alidnsDomainAttachmentResource{}
	_ resource.ResourceWithConfigure = &alidnsDomainAttachmentResource{}
)

func NewAlidnsDomainAttachmentResource() resource.Resource {
	return &alidnsDomainAttachmentResource{}
}

type alidnsDomainAttachmentResource struct {
	client *alicloudDnsClient.Client
}

type alidnsDomainAttachmentResourceModel struct {
	InstanceId types.String `tfsdk:"instance_id"`
	Domain     types.String `tfsdk:"domain"`
}

func (r *alidnsDomainAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alidns_domain_attachment"
}

func (r *alidnsDomainAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"instance_id": schema.StringAttribute{
				Description: "Instance Domain Id.",
				Required:    true,
			},
			"domain": schema.StringAttribute{
				Description: "Domain to bind to instance domain.",
				Required:    true,
			},
		},
	}
}

func (r *alidnsDomainAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(alicloudClients).dnsClient
}

func (r *alidnsDomainAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *alidnsDomainAttachmentResourceModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	//////////////////////// DATA VALIDATION ////////////////////////
	if plan.Domain.ValueString() == "" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Bind Domain Instance",
			"domain cannot be empty.",
		)
		return
	}

	if plan.InstanceId.ValueString() == "" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Bind Domain Instance",
			"instance_id cannot be empty.",
		)
		return
	}

	bindInstanceDiags := r.createBindInstance(plan)
	resp.Diagnostics.Append(bindInstanceDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := &alidnsDomainAttachmentResourceModel{}
	state.InstanceId = plan.InstanceId
	state.Domain = plan.Domain

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *alidnsDomainAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *alidnsDomainAttachmentResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

		readDomainRecord := func() (err error) {
		runtime := &util.RuntimeOptions{}

		describeDomainInfoWithDomainRequest := &alicloudDnsClient.DescribeDomainInfoRequest{
			DomainName: tea.String(state.Domain.ValueString()),
		}

		dnsResp, err := r.client.DescribeDomainInfoWithOptions(describeDomainInfoWithDomainRequest, runtime)
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

		// Remove terraform state if existing binding is not found
		// This will make sure terraform rebind domain correctly
		if dnsResp.Body.InstanceId == nil {
			resp.State.RemoveResource(ctx)
			return
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(readDomainRecord, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to get domain info.",
			err.Error(),
		)
		return
	}
}

func (r *alidnsDomainAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state *alidnsDomainAttachmentResourceModel
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

	//////////////////////// DATA VALIDATION ////////////////////////
	if plan.Domain.ValueString() == "" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Bind Domain Instance",
			"domain cannot be empty.",
		)
		return
	}

	if plan.InstanceId.ValueString() == "" {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Bind Domain Instance",
			"instance_id cannot be empty.",
		)
		return
	}

	bindInstanceDiags := r.createBindInstance(plan)
	resp.Diagnostics.Append(bindInstanceDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state = plan

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *alidnsDomainAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *alidnsDomainAttachmentResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	removeBindInstanceDiags := r.removeBindInstance(state)
	resp.Diagnostics.Append(removeBindInstanceDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *alidnsDomainAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}

func (r *alidnsDomainAttachmentResource) createBindInstance(plan *alidnsDomainAttachmentResourceModel) diag.Diagnostics {
	bindInstanceRecord := func() error {
		runtime := &util.RuntimeOptions{}

		bindInstanceDomainsWithIdRequest := &alicloudDnsClient.BindInstanceDomainsRequest{
			InstanceId:  tea.String(plan.InstanceId.ValueString()),
			DomainNames: tea.String(plan.Domain.ValueString()),
		}

		if _, err := r.client.BindInstanceDomainsWithOptions(bindInstanceDomainsWithIdRequest, runtime); err != nil {
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
	err := backoff.Retry(bindInstanceRecord, reconnectBackoff)
	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Failed to bind domain to instance.",
				err.Error(),
			),
		}
	}
	return nil
}

func (r *alidnsDomainAttachmentResource) removeBindInstance(state *alidnsDomainAttachmentResourceModel) diag.Diagnostics {
	unbindInstanceRecord := func() error {
		runtime := &util.RuntimeOptions{}

		unbindInstanceDomainsRequest := &alicloudDnsClient.UnbindInstanceDomainsRequest{
			InstanceId:  tea.String(state.InstanceId.ValueString()),
			DomainNames: tea.String(state.Domain.ValueString()),
		}

		if _, err := r.client.UnbindInstanceDomainsWithOptions(unbindInstanceDomainsRequest, runtime); err != nil {
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
	err := backoff.Retry(unbindInstanceRecord, reconnectBackoff)
	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Failed to bind domain to instance.",
				err.Error(),
			),
		}
	}
	return nil
}
