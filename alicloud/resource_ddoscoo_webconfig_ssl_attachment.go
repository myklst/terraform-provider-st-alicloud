package alicloud

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudAntiddosClient "github.com/alibabacloud-go/ddoscoo-20200101/v2/client"
)

var (
	_ resource.Resource              = &ddoscooWebconfigSslAttachmentResource{}
	_ resource.ResourceWithConfigure = &ddoscooWebconfigSslAttachmentResource{}
)

func NewDdosCooWebconfigSslAttachmentResource() resource.Resource {
	return &ddoscooWebconfigSslAttachmentResource{}
}

type ddoscooWebconfigSslAttachmentResource struct {
	client *alicloudAntiddosClient.Client
}

type ddoscooWebconfigSslAttachmentModel struct {
	Domain types.String `tfsdk:"domain"`
	CertId types.Int64  `tfsdk:"cert_id"`
}

// Metadata returns the SSL binding resource name.
func (r *ddoscooWebconfigSslAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ddoscoo_webconfig_ssl_attachment"
}

// Schema defines the schema for the SSL certificate binding resource.
func (r *ddoscooWebconfigSslAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Associates a domain with a SSL cert in Anti-DDoS website configuration.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Description: "Domain name.",
				Required:    true,
			},
			"cert_id": schema.Int64Attribute{
				Description: "SSL Certificate ID.",
				Required:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ddoscooWebconfigSslAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).antiddosClient
}

// Create a new SSL cert and domain binding
func (r *ddoscooWebconfigSslAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *ddoscooWebconfigSslAttachmentModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Bind SSL cert with domain
	err := r.bindCert(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to bind SSL cert.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &ddoscooWebconfigSslAttachmentModel{
		Domain: plan.Domain,
		CertId: plan.CertId,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read web rules configuration for SSL cert and domain binding
func (r *ddoscooWebconfigSslAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *ddoscooWebconfigSslAttachmentModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Retry backoff function
	readWebRules := func() error {
		runtime := &util.RuntimeOptions{}

		describeWebRulesRequest := &alicloudAntiddosClient.DescribeWebRulesRequest{
			PageSize: tea.Int32(1),
			Domain:   tea.String(state.Domain.ValueString()),
		}

		webRulesResponse, err := r.client.DescribeWebRulesWithOptions(describeWebRulesRequest, runtime)
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

		if *webRulesResponse.Body.TotalCount > 0 {
			if webRulesResponse.Body.WebRules[0].CertName != nil && *webRulesResponse.Body.WebRules[0].CertName != "" {
				// Convert ID "<id>.pem" to int64 format
				certIdString := *webRulesResponse.Body.WebRules[0].CertName
				certId, _ := strconv.ParseInt(strings.TrimSuffix(certIdString, ".pem"), 10, 64)
				state.CertId = types.Int64Value(certId)
			} else {
				state.CertId = types.Int64Null()
			}

			state.Domain = types.StringValue(*webRulesResponse.Body.WebRules[0].Domain)

			// Set refreshed state
			setStateDiags := resp.State.Set(ctx, &state)
			resp.Diagnostics.Append(setStateDiags...)
			if resp.Diagnostics.HasError() {
				resp.Diagnostics.AddError(
					"[API ERROR] Failed to Set Read Website Configuration Rules to State",
					err.Error(),
				)
			}

		} else {
			resp.State.RemoveResource(ctx)
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second

	err := backoff.Retry(readWebRules, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Read domain and SSL cert",
			err.Error(),
		)
		return
	}

}

// Update binds new SSL cert to domain and sets the updated Terraform state on success.
func (r *ddoscooWebconfigSslAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *ddoscooWebconfigSslAttachmentModel

	// Retrieve values from plan
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Bind SSL cert to domain
	err := r.bindCert(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Update SSL Cert Binding",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &ddoscooWebconfigSslAttachmentModel{
		Domain: plan.Domain,
		CertId: plan.CertId,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// SSL cert could not be unbinded, will always remain.
func (r *ddoscooWebconfigSslAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *ddoscooWebconfigSslAttachmentModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Function to bind certificate to domain
func (r *ddoscooWebconfigSslAttachmentResource) bindCert(plan *ddoscooWebconfigSslAttachmentModel) error {
	bindSSLCert := func() error {
		runtime := &util.RuntimeOptions{}

		associateWebCertRequest := &alicloudAntiddosClient.AssociateWebCertRequest{
			Domain: tea.String(plan.Domain.ValueString()),
			CertId: tea.Int32(int32(plan.CertId.ValueInt64())),
		}

		_, _err := r.client.AssociateWebCertWithOptions(associateWebCertRequest, runtime)
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

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(bindSSLCert, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}
