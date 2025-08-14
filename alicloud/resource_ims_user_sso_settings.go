package alicloud

import (
	"context"
	"fmt"
	"time"

	alicloudImsClient "github.com/alibabacloud-go/ims-20190815/v4/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &userSSOSettingsResource{}
	_ resource.ResourceWithConfigure = &userSSOSettingsResource{}
)

func NewUserSSOSettingsResource() resource.Resource {
	return &userSSOSettingsResource{}
}

type userSSOSettingsResource struct {
	client *alicloudImsClient.Client
}

type userSSOSettingsResourceModel struct {
	SSOEnabled         types.Bool   `tfsdk:"sso_enabled"`
	MetadataDocument   types.String `tfsdk:"metadata_document"`
	AuxiliaryDomain    types.String `tfsdk:"auxiliary_domain"`
	SSOLoginWithDomain types.Bool   `tfsdk:"sso_login_with_domain"`
}

func (r *userSSOSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ims_user_sso_settings"
}

func (r *userSSOSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the SSO (Single Sign-On) settings for a user, including enabling SSO, specifying the metadata document, and configuring login behavior with a custom domain.",
		Attributes: map[string]schema.Attribute{
			"sso_enabled": schema.BoolAttribute{
				Description: "Whether SSO is enabled for the user account. Set to `true` to require Single Sign-On for authentication.",
				Required:    true,
			},
			"metadata_document": schema.StringAttribute{
				Description: "The Base64-encoded SAML metadata document provided by the identity provider (IdP) for SSO configuration.",
				Required:    true,
			},
			"sso_login_with_domain": schema.BoolAttribute{
				Description: "Indicates whether users can log in using their custom domain name instead of the default tenant domain.",
				Required:    true,
			},
			"auxiliary_domain": schema.StringAttribute{
				Description: "The custom auxiliary domain name associated with the SSO configuration, used for login and routing authentication requests.",
				Required:    true,
			},
		},
	}
}

func (r *userSSOSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).imsClient
}

func (r *userSSOSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *userSSOSettingsResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setUserSsoSettings(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to set the User SSO settings.",
			err.Error(),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *userSSOSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *userSSOSettingsResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	GetUserSsoSettings, err := r.client.GetUserSsoSettings()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read User SSO Settings",
			fmt.Sprintf("Error calling GetUserSsoSettings: %s", err),
		)
		return
	}

	sso := GetUserSsoSettings.Body.UserSsoSettings
	state.MetadataDocument = types.StringValue(*sso.MetadataDocument)
	state.SSOEnabled = types.BoolValue(*sso.SsoEnabled)
	state.SSOLoginWithDomain = types.BoolValue(*sso.SsoLoginWithDomain)

	if sso.AuxiliaryDomain != nil {
		state.AuxiliaryDomain = types.StringValue(*sso.AuxiliaryDomain)
	} else {
		state.AuxiliaryDomain = types.StringNull()
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *userSSOSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	getUserSsoSettings, err := r.client.GetUserSsoSettings()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve SSO settings",
			fmt.Sprintf("Error calling GetUserSsoSettings: %s", err),
		)
		return
	}

	userSsoSettingsResponse := getUserSsoSettings.Body.UserSsoSettings
	if userSsoSettingsResponse == nil || userSsoSettingsResponse.AuxiliaryDomain == nil {
		resp.Diagnostics.AddError(
			"Invalid Response",
			"No AuxiliaryDomain found in API response",
		)
		return
	}

	auxiliaryDomain := *userSsoSettingsResponse.AuxiliaryDomain
	if req.ID != auxiliaryDomain {
		resp.Diagnostics.AddError(
			"Domain mismatch",
			fmt.Sprintf("Expected domain %q from AliCloud, but got %q from import command", auxiliaryDomain, req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("auxiliary_domain"), auxiliaryDomain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("sso_enabled"), userSsoSettingsResponse.SsoEnabled)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metadata_document"), userSsoSettingsResponse.MetadataDocument)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("sso_login_with_domain"), userSsoSettingsResponse.SsoLoginWithDomain)...)
}

func (r *userSSOSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *userSSOSettingsResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setUserSsoSettings(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to SetUserSSO.",
			err.Error(),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *userSSOSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan *userSSOSettingsResourceModel
	getPlanDiags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// On terraform destroy, disable the User SSO settings (singleton resource) instead of deleting it.
	plan.SSOEnabled = types.BoolValue(false)

	if err := r.setUserSsoSettings(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to SetUserSSO.",
			err.Error(),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *userSSOSettingsResource) setUserSsoSettings(plan *userSSOSettingsResourceModel) (err error) {
	if r.client == nil {
		return fmt.Errorf("client is not initialized in userSSOSettingsResource")
	}

	setUserSsoSettingsRequest := &alicloudImsClient.SetUserSsoSettingsRequest{
		MetadataDocument:   tea.String(plan.MetadataDocument.ValueString()),
		AuxiliaryDomain:    tea.String(plan.AuxiliaryDomain.ValueString()),
		SsoEnabled:         tea.Bool(plan.SSOEnabled.ValueBool()),
		SsoLoginWithDomain: tea.Bool(plan.SSOLoginWithDomain.ValueBool()),
	}

	setUserSsoSettings := func() error {
		runtime := &util.RuntimeOptions{}

		if _, err := r.client.SetUserSsoSettingsWithOptions(setUserSsoSettingsRequest, runtime); err != nil {
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
	return backoff.Retry(setUserSsoSettings, reconnectBackoff)
}
