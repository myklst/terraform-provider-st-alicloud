package alicloud

import (
	"context"
	"time"
	"fmt"

	"github.com/cenkalti/backoff/v4"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	alicloudAntiddosClient "github.com/alibabacloud-go/ddoscoo-20200101/v2/client"
)

var (
	_ resource.Resource              = &ddoscooWebAIProtectConfigResource{}
	_ resource.ResourceWithConfigure = &ddoscooWebAIProtectConfigResource{}
)

func NewDdosCooWebAIProtectConfigResource() resource.Resource {
	return &ddoscooWebAIProtectConfigResource{}
}

type ddoscooWebAIProtectConfigResource struct {
	client *alicloudAntiddosClient.Client
}

type ddoscooWebAIProtectConfigModel struct {
	Enabled types.Bool  `tfsdk:"enabled"`
	Domain types.String `tfsdk:"domain"`
	Mode types.String   `tfsdk:"mode"`
	Level types.String  `tfsdk:"level"`
}

// Metadata returns the web ai protect mode configuration resource name.
func (r *ddoscooWebAIProtectConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ddoscoo_webconfig_ai_protect_config"
}

// Schema defines the schema for the web ai protect mode configuration resource.
func (r *ddoscooWebAIProtectConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Modify a domain AI Protect Mode in Anti-DDoS website configuration.",
		Attributes: map[string]schema.Attribute{
			"enabled": schema.BoolAttribute{
				Description: "Enable/Disable of ai protect mode status.",
				Required:    true,
			},
			"domain": schema.StringAttribute{
				Description: "Domain name.",
				Required:    true,
			},
			"mode": schema.StringAttribute{
				Description: "config to set AiMode.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("warning", "protection"),
				},
				Default: stringdefault.StaticString("protection"),
			},
			"level": schema.StringAttribute{
				Description: "config to set AiTemplate.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("loose", "normal", "strict"),
				},
				Default: stringdefault.StaticString("normal"),
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ddoscooWebAIProtectConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).antiddosClient
}

// Create a modify web ai protect mode configuration.
func (r *ddoscooWebAIProtectConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *ddoscooWebAIProtectConfigModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Modify Web AI Protect Mode.
	err := r.modifyAIProtectMode(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to modify Antiddos AI protection Mode.",
			err.Error(),
		)
		return
	}

	// Set state items.
	state := &ddoscooWebAIProtectConfigModel{
		Enabled: plan.Enabled,
		Domain: plan.Domain,
		Mode: plan.Mode,
		Level: plan.Level,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read web ai protect configuration for domain.
func (r *ddoscooWebAIProtectConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state *ddoscooWebAIProtectConfigModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Retry backoff function.
	readWebAIProtectMode := func() error {
		runtime := &util.RuntimeOptions{}

		describeWebCcProtectSwitchRequest := &alicloudAntiddosClient.DescribeWebCcProtectSwitchRequest{
			Domains:  []*string{tea.String(state.Domain.ValueString())},
		}

		webCcProtectSwitch, err := r.client.DescribeWebCcProtectSwitchWithOptions(describeWebCcProtectSwitchRequest, runtime)
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

		if len(webCcProtectSwitch.Body.ProtectSwitchList)  > 0 {
			//convert from aliyun antiddos web ai protect sdk AiRuleEnable keyword to readable variable (Enabled).
			switch *webCcProtectSwitch.Body.ProtectSwitchList[0].AiRuleEnable {
			case 0:
				state.Enabled = types.BoolValue(false)
			case 1:
				state.Enabled = types.BoolValue(true)
			}

			//convert from aliyun antiddos web ai protect sdk AiMode keyword to readable variable (Mode).
			switch *webCcProtectSwitch.Body.ProtectSwitchList[0].AiMode {
			case "watch":
				state.Mode = types.StringValue("warning")
			case "defense":
				state.Mode = types.StringValue("protection")
			}

			//convert from aliyun antiddos web ai protect sdk AiTemplate keyword to readable variable (Level).
			switch *webCcProtectSwitch.Body.ProtectSwitchList[0].AiTemplate {
			case "level30":
				state.Level = types.StringValue("loose")
			case "level60":
				state.Level = types.StringValue("normal")
			case "level90":
				state.Level = types.StringValue("strict")
			}

			state.Domain = types.StringValue(*webCcProtectSwitch.Body.ProtectSwitchList[0].Domain)

			// Set refreshed state
			setStateDiags := resp.State.Set(ctx, &state)
			resp.Diagnostics.Append(setStateDiags...)
			if resp.Diagnostics.HasError() {
				resp.Diagnostics.AddError(
					"[API ERROR] Failed to Set ANtiddos AI Protection Website Configuration Mode to State",
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
	reconnectBackoff.MaxElapsedTime = 60 * time.Second

	err := backoff.Retry(readWebAIProtectMode, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Read Antiddos AI Protection Mode",
			err.Error(),
		)
		return
	}

}

// Update web ai protect configuration and sets the updated Terraform state on success.
func (r *ddoscooWebAIProtectConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *ddoscooWebAIProtectConfigModel

	// Retrieve values from plan
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Modify Web AI Protect Mode
	err := r.modifyAIProtectMode(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Update modify Antiddos AI protection Mode.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &ddoscooWebAIProtectConfigModel{
		Enabled: plan.Enabled,
		Domain: plan.Domain,
		Mode: plan.Mode,
		Level: plan.Level,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// SSL cert could not be unbinded, will always remain.
func (r *ddoscooWebAIProtectConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *ddoscooWebAIProtectConfigModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Function to modify AI Protection Mode for domain
func (r *ddoscooWebAIProtectConfigResource) modifyAIProtectMode(plan *ddoscooWebAIProtectConfigModel) error {
	level   := plan.Level.ValueString()
	mode    := plan.Mode.ValueString()
	enabled := map[bool]int{false: 0, true: 1}[plan.Enabled.ValueBool()]

	enableAIProtectConfig := func() error {
		runtime := &util.RuntimeOptions{}

		// enable/disable antiddos web ai protect configuration
		modifyWebAIProtectSwitchRequest := &alicloudAntiddosClient.ModifyWebAIProtectSwitchRequest{
			Config: tea.String(fmt.Sprintf("{\"AiRuleEnable\": %d}",enabled)),
			Domain: tea.String(plan.Domain.ValueString()),
		}

		_, _err := r.client.ModifyWebAIProtectSwitchWithOptions(modifyWebAIProtectSwitchRequest, runtime)
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

	modifyAIProtectConfig := func() error {
		runtime := &util.RuntimeOptions{}

			//convert input (level) to aliyun antiddos web ai protect sdk AiTemplate needed keyword ("level30"/"level60"/"level90").
			switch level {
			case "loose":
				level = "level30"
			case "normal":
				level = "level60"
			case "strict":
				level = "level90"
			}

			//convert input (mode) to aliyun antiddos web ai protect sdk AiMode needed keyword ("watch"/"defense").
			switch mode {
			case "warning":
				mode = "watch"
			case "protection":
				mode = "defense"
			}

			// modify antiddos web ai protect mode configuration
			modifyWebAIProtectModeRequest := &alicloudAntiddosClient.ModifyWebAIProtectModeRequest{
				Domain: tea.String(plan.Domain.ValueString()),
				Config: tea.String(fmt.Sprintf("{\"AiTemplate\":\"%s\",\"AiMode\":\"%s\"}", level, mode)),
			}

			_, _err := r.client.ModifyWebAIProtectModeWithOptions(modifyWebAIProtectModeRequest, runtime)
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
	reconnectBackoff.MaxElapsedTime = 60 * time.Second
	err := backoff.Retry(enableAIProtectConfig, reconnectBackoff)
	if err != nil {
		return err
	}

	reconnectBackoff.MaxElapsedTime = 60 * time.Second
	err = backoff.Retry(modifyAIProtectConfig, reconnectBackoff)
	if err != nil {
		return err
	}
	return nil
}
