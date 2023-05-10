package alicloud

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudDnsClient "github.com/alibabacloud-go/alidns-20150109/v4/client"
)

var (
	_ resource.Resource                = &aliDnsRecordWeightResource{}
	_ resource.ResourceWithConfigure   = &aliDnsRecordWeightResource{}
	_ resource.ResourceWithImportState = &aliDnsRecordWeightResource{}
	_ resource.ResourceWithModifyPlan  = &aliDnsRecordWeightResource{}
)

func NewAliDnsRecordWeightResource() resource.Resource {
	return &aliDnsRecordWeightResource{}
}

type aliDnsRecordWeightResource struct {
	client *alicloudDnsClient.Client
}

type aliDnsRecordWeightResourceModel struct {
	Id     types.String `tfsdk:"id"`
	Weight types.Int64  `tfsdk:"weight"`
	Status types.Bool   `tfsdk:"status"`
}

// Metadata returns the resource DNS weight type name.
func (r *aliDnsRecordWeightResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alidns_record_weight"
}

// Schema defines the schema for the DNS weight resource.
func (r *aliDnsRecordWeightResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Alidns record weight resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Subdomain Record Id.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"weight": schema.Int64Attribute{
				Description: "Subdomain Weight.",
				Required:    true,
			},
			"status": schema.BoolAttribute{
				Description: "Subdomain Weight Status",
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *aliDnsRecordWeightResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).dnsClient
}

// Create a new DNS weight resource
func (r *aliDnsRecordWeightResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *aliDnsRecordWeightResourceModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set Weight of SubDomain
	err := r.setWeight(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Set DNS Domain Weight",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &aliDnsRecordWeightResourceModel{}
	state.Id = plan.Id
	state.Weight = plan.Weight
	state.Status = plan.Status

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read DNS weight resource information
func (r *aliDnsRecordWeightResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *aliDnsRecordWeightResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Retry backoff function
	readRecordWeight := func() error {
		runtime := &util.RuntimeOptions{}

		//Look SubDomain Name
		DescDomainRecordWithIdRequest := &alicloudDnsClient.DescribeDomainRecordInfoRequest{
			RecordId: tea.String(state.Id.ValueString()),
		}

		responseById, err := r.client.DescribeDomainRecordInfoWithOptions(DescDomainRecordWithIdRequest, runtime)
		if err != nil {
			if _t, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*_t.Code) {
					return err
				} else {
					if *_t.Code == "InvalidRR.NoExist" {
						return backoff.Permanent(fmt.Errorf("domain record not found"))
					}
					return backoff.Permanent(err)
				}
			} else {
				return err
			}
		}

		// Combine Domain Name and Resource Record (RR) for SubDomain Name
		subdomainName := fmt.Sprintf("%s.%s", *responseById.Body.RR, *responseById.Body.DomainName)
		fmt.Println(subdomainName)

		// Look for SubDomain Weight
		DescSubDomainRecords := &alicloudDnsClient.DescribeSubDomainRecordsRequest{
			SubDomain: tea.String(subdomainName),
			PageSize:  tea.Int64(100),
		}

		responseBySubRecords, err := r.client.DescribeSubDomainRecordsWithOptions(DescSubDomainRecords, runtime)
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

		// Look for SubDomain Status
		DescDNSSLBSubDomains := &alicloudDnsClient.DescribeDNSSLBSubDomainsRequest{
			DomainName: tea.String(*responseById.Body.DomainName),
			PageSize:   tea.Int64(100),
		}

		responseBySLBStatus, err := r.client.DescribeDNSSLBSubDomainsWithOptions(DescDNSSLBSubDomains, runtime)
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

		// Set new info if there's changes
		// Record Weight and Status based on SubDomain RecordId
		for _, record := range responseBySubRecords.Body.DomainRecords.Record {
			if state.Id.ValueString() == *record.RecordId {
				state.Weight = types.Int64Value(int64(*record.Weight))
			}
		}
		// Record Status
		for _, subDomain := range responseBySLBStatus.Body.SlbSubDomains.SlbSubDomain {
			if *subDomain.SubDomain == subdomainName {
				state.Status = types.BoolValue(*subDomain.Open)
			}
		}

		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second

	err := backoff.Retry(readRecordWeight, reconnectBackoff)
	if err != nil {
		if err.Error() == "domain record not found" {
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to Read DNS Record Weight",
				err.Error(),
			)
		}
		return
	}

	// Set refreshed state
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the DNS weight resource and sets the updated Terraform state on success.
func (r *aliDnsRecordWeightResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *aliDnsRecordWeightResourceModel

	// Retrieve values from plan
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set Weight of SubDomain
	err := r.setWeight(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Set DNS Domain Weight",
			err.Error(),
		)
		return
	}

	// Set state values
	state := &aliDnsRecordWeightResourceModel{}
	state.Id = plan.Id
	state.Weight = plan.Weight
	state.Status = plan.Status

	// Set state to plan data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the DNS weight resource and removes the Terraform state on success.
func (r *aliDnsRecordWeightResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *aliDnsRecordWeightResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *aliDnsRecordWeightResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Retrieve import RecordId and save to id attribute
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *aliDnsRecordWeightResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If the entire plan is null, the resource is planned for destruction.
	if !(req.Plan.Raw.IsNull()) {
		var plan *aliDnsRecordWeightResourceModel
		getPlanDiags := req.Plan.Get(ctx, &plan)
		resp.Diagnostics.Append(getPlanDiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Set plan status to true whenever CRUD is ran
		plan.Status = types.BoolValue(true)

		resp.Plan.Set(ctx, &plan)
		if resp.Diagnostics.HasError() {
			return
		}
	}
}

func (r *aliDnsRecordWeightResource) setWeight(plan *aliDnsRecordWeightResourceModel) error {
	setRecordWeight := func() error {
		runtime := &util.RuntimeOptions{}

		// Look for SubDomain Name
		DescDomainRecordWithIdRequest := &alicloudDnsClient.DescribeDomainRecordInfoRequest{
			RecordId: tea.String(plan.Id.ValueString()),
		}

		responseById, err := r.client.DescribeDomainRecordInfoWithOptions(DescDomainRecordWithIdRequest, runtime)
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

		// Look for Subdomain Statuses
		DescDnsSlbSubDomainRequest := &alicloudDnsClient.DescribeDNSSLBSubDomainsRequest{
			DomainName: tea.String(*responseById.Body.DomainName),
		}

		responseByName, err := r.client.DescribeDNSSLBSubDomainsWithOptions(DescDnsSlbSubDomainRequest, runtime)
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

		// Combine Domain Name and Resource Record (RR) for SubDomain Name
		subdomainName := fmt.Sprintf("%s.%s", *responseById.Body.RR, *responseById.Body.DomainName)

		for _, subdomains := range responseByName.Body.SlbSubDomains.SlbSubDomain {
			if subdomainName == *subdomains.SubDomain && !*subdomains.Open {

				// Enable Weight settings
				setDNSSLBStatusRequest := &alicloudDnsClient.SetDNSSLBStatusRequest{
					SubDomain: tea.String(subdomainName),
					Open:      tea.Bool(true),
				}

				_, err = r.client.SetDNSSLBStatusWithOptions(setDNSSLBStatusRequest, runtime)
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
			}
		}

		// Update Weight of records
		updateDNSSLBWeightRequest := &alicloudDnsClient.UpdateDNSSLBWeightRequest{
			RecordId: tea.String(plan.Id.ValueString()),
			Weight:   tea.Int32(int32(plan.Weight.ValueInt64())),
		}

		_, err = r.client.UpdateDNSSLBWeightWithOptions(updateDNSSLBWeightRequest, runtime)
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
	err := backoff.Retry(setRecordWeight, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}
