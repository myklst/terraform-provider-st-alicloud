package alicloud

import (
	"context"
	"fmt"
	"time"

	alicloudImsClient "github.com/alibabacloud-go/ims-20190815/v4/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
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
	SsoEnabled         types.Bool   `tfsdk:"sso_enabled"`
	MetadataDocument   types.String `tfsdk:"metadata_document"`
	SsoLoginWithDomain types.Bool   `tfsdk:"sso_login_with_domain"`
	AuxiliaryDomain    types.String `tfsdk:"auxiliary_domain"`
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
	var state userSSOSettingsResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readUserSsoSettings := func() error {
		getUserSsoSettings, err := r.client.GetUserSsoSettings()
		if err != nil {
			if sdkErr, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*sdkErr.Code) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}

		sso := getUserSsoSettings.Body.UserSsoSettings
		state.MetadataDocument = types.StringValue(*sso.MetadataDocument)
		state.SsoLoginWithDomain = types.BoolValue(*sso.SsoLoginWithDomain)
		state.SsoEnabled = types.BoolValue(*sso.SsoEnabled)

		if *sso.SsoLoginWithDomain {
			state.AuxiliaryDomain = types.StringValue(*sso.AuxiliaryDomain)
		} else {
			state.AuxiliaryDomain = types.StringValue("")
		}

		return nil
	}

	retryBackoff := backoff.NewExponentialBackOff()
	retryBackoff.MaxElapsedTime = 30 * time.Second
	if err := backoff.Retry(readUserSsoSettings, retryBackoff); err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read User SSO Settings",
			fmt.Sprintf("Error calling GetUserSsoSettings: %s", err),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
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
	plan.SsoEnabled = types.BoolValue(false)

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

	// To successfully set SsoLoginWithDomain to false, AuxiliaryDomain must first be cleared.
	var auxiliaryDomain *string
	if plan.SsoLoginWithDomain.ValueBool() {
		auxiliaryDomain = tea.String(plan.AuxiliaryDomain.ValueString())
	} else {
		auxiliaryDomain = tea.String("")
	}

	setUserSsoSettingsRequest := &alicloudImsClient.SetUserSsoSettingsRequest{
		SsoEnabled:         tea.Bool(plan.SsoEnabled.ValueBool()),
		MetadataDocument:   tea.String(plan.MetadataDocument.ValueString()),
		SsoLoginWithDomain: tea.Bool(plan.SsoLoginWithDomain.ValueBool()),
		AuxiliaryDomain:    auxiliaryDomain,
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
