package alicloud

import (
	"context"
	"fmt"
	"time"

	util "github.com/alibabacloud-go/tea-utils/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudKvstoreClient "github.com/alibabacloud-go/r-kvstore-20150101/client"
	openapiClient "github.com/alibabacloud-go/darabonba-openapi/client"
)

var (
	_ resource.Resource                = &kvstoreInstanceBandwidthBurstResource{}
	_ resource.ResourceWithConfigure   = &kvstoreInstanceBandwidthBurstResource{}
	_ resource.ResourceWithImportState = &kvstoreInstanceBandwidthBurstResource{}
)

func NewKvstoreInstanceBandwidthBurstResource() resource.Resource {
	return &kvstoreInstanceBandwidthBurstResource{}
}

type kvstoreInstanceBandwidthBurstResource struct {
	client *alicloudKvstoreClient.Client
}

type kvstoreInstanceBandwidthBurstModel struct {
	Id         types.String `tfsdk:"id"`
	InstanceId types.String `tfsdk:"instance_id"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

func (r *kvstoreInstanceBandwidthBurstResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kvstore_instance_bandwidth_burst"
}

func (r *kvstoreInstanceBandwidthBurstResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Enables or disables elastic bandwidth burst for an Alibaba Cloud Redis (R-Kvstore) instance. " +
			"Burst is an instance-level setting — when enabled, the instance can temporarily exceed its bandwidth limit.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Same as instance_id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"instance_id": schema.StringAttribute{
				Description: "The ID of the Redis instance.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether to enable bandwidth burst. Set to true to enable, false to disable.",
				Required:    true,
			},
		},
	}
}

func (r *kvstoreInstanceBandwidthBurstResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).kvstoreClient
}

func (r *kvstoreInstanceBandwidthBurstResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *kvstoreInstanceBandwidthBurstModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.setBurst(plan.InstanceId.ValueString(), plan.Enabled.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to set Redis bandwidth burst.",
			err.Error(),
		)
		return
	}

	state := &kvstoreInstanceBandwidthBurstModel{
		Id:         plan.InstanceId,
		InstanceId: plan.InstanceId,
		Enabled:    plan.Enabled,
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreInstanceBandwidthBurstResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *kvstoreInstanceBandwidthBurstModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	instanceId := state.InstanceId.ValueString()

	burstEnabled, err := r.readBurst(instanceId)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read Redis intranet attribute.",
			err.Error(),
		)
		return
	}

	// If burst is disabled, the resource is gone
	if !burstEnabled {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Id = types.StringValue(instanceId)
	state.Enabled = types.BoolValue(true)

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreInstanceBandwidthBurstResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *kvstoreInstanceBandwidthBurstModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.setBurst(plan.InstanceId.ValueString(), plan.Enabled.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to update Redis bandwidth burst.",
			err.Error(),
		)
		return
	}

	state := &kvstoreInstanceBandwidthBurstModel{
		Id:         plan.InstanceId,
		InstanceId: plan.InstanceId,
		Enabled:    plan.Enabled,
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreInstanceBandwidthBurstResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *kvstoreInstanceBandwidthBurstModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.setBurst(state.InstanceId.ValueString(), false)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to disable Redis bandwidth burst.",
			err.Error(),
		)
		return
	}
}

func (r *kvstoreInstanceBandwidthBurstResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("instance_id"), req, resp)
}

// readBurst reads the current burst status via DescribeIntranetAttribute.
// The Go SDK v1.0.7 doesn't expose IntranetBandWidthBurst in the response struct,
// so we use CallApi to get the raw response.
func (r *kvstoreInstanceBandwidthBurstResource) readBurst(instanceId string) (bool, error) {
	var burstEnabled bool

	readBurst := func() error {
		runtime := &util.RuntimeOptions{}
		params := &openapiClient.Params{
			Action:      tea.String("DescribeIntranetAttribute"),
			Version:     tea.String("2015-01-01"),
			Protocol:    tea.String("HTTPS"),
			Pathname:    tea.String("/"),
			Method:      tea.String("POST"),
			AuthType:    tea.String("AK"),
			BodyType:    tea.String("Json"),
			ReqBodyType: tea.String("Query"),
			Style:       tea.String("RPC"),
		}
		openapiReq := &openapiClient.OpenApiRequest{
			Query: map[string]*string{
				"InstanceId": tea.String(instanceId),
			},
		}
		result, err := r.client.CallApi(params, openapiReq, runtime)
		if err != nil {
			if t, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*t.Code) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}

		// Parse IntranetBandWidthBurst from raw response
		if result != nil {
			if burstVal, ok := result["IntranetBandWidthBurst"]; ok {
				if s, ok := burstVal.(string); ok {
					burstEnabled = s == "true" || s == "True" || s == "1"
				}
			}
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(readBurst, reconnectBackoff)
	if err != nil {
		return false, fmt.Errorf("failed to read burst status for instance %s: %w", instanceId, err)
	}
	return burstEnabled, nil
}

// setBurst enables or disables bandwidth burst via EnableAdditionalBandwidth API.
// The Go SDK v1.0.7 doesn't expose BandWidthBurst in the request struct,
// so we use CallApi to pass it as a raw query parameter.
func (r *kvstoreInstanceBandwidthBurstResource) setBurst(instanceId string, enabled bool) error {
	burstStr := "false"
	if enabled {
		burstStr = "true"
	}

	enableBurst := func() error {
		runtime := &util.RuntimeOptions{}
		params := &openapiClient.Params{
			Action:      tea.String("EnableAdditionalBandwidth"),
			Version:     tea.String("2015-01-01"),
			Protocol:    tea.String("HTTPS"),
			Pathname:    tea.String("/"),
			Method:      tea.String("POST"),
			AuthType:    tea.String("AK"),
			BodyType:    tea.String("Json"),
			ReqBodyType: tea.String("Query"),
			Style:       tea.String("RPC"),
		}
		openapiReq := &openapiClient.OpenApiRequest{
			Query: map[string]*string{
				"InstanceId":      tea.String(instanceId),
				"BandWidthBurst":  tea.String(burstStr),
				"ChargeType":      tea.String("PostPaid"),
			},
		}
		_, err := r.client.CallApi(params, openapiReq, runtime)
		if err != nil {
			if t, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*t.Code) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(enableBurst, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to set bandwidth burst for instance %s: %w", instanceId, err)
	}
	return nil
}
