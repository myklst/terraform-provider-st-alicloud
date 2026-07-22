package alicloud

import (
	"context"
	"fmt"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudOpenapiClient "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	openapiutil "github.com/alibabacloud-go/openapi-util/service"
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
	client *alicloudOpenapiClient.Client
	region string
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
	r.client = req.ProviderData.(alicloudClients).kvstoreRawClient
	r.region = req.ProviderData.(alicloudClients).region
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

// readBurst reads the current burst status.
// Primary: DescribeIntranetAttribute → IntranetBandWidthBurst (not always returned)
// Fallback: DescribeRoleZoneInfo → IsOpenBandWidthService on any node
func (r *kvstoreInstanceBandwidthBurstResource) readBurst(instanceId string) (bool, error) {
	var burstEnabled bool

	readBurst := func() error {
		runtime := &util.RuntimeOptions{}
		params := &alicloudOpenapiClient.Params{
			Action:      tea.String("DescribeIntranetAttribute"),
			Version:     tea.String("2015-01-01"),
			Protocol:    tea.String("HTTPS"),
			Pathname:    tea.String("/"),
			Method:      tea.String("POST"),
			AuthType:    tea.String("AK"),
			BodyType:    tea.String("json"),
			ReqBodyType: tea.String("json"),
			Style:       tea.String("RPC"),
		}
		queries := map[string]any{}
		queries["InstanceId"] = tea.String(instanceId)
		openapiReq := &alicloudOpenapiClient.OpenApiRequest{
			Query: openapiutil.Query(queries),
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

		// Parse IntranetBandWidthBurst from raw response (nested under "body")
		if result != nil {
			if body, ok := result["body"].(map[string]any); ok {
				if burstVal, ok := body["IntranetBandWidthBurst"]; ok {
					if s, ok := burstVal.(string); ok {
						burstEnabled = s == "true" || s == "True" || s == "1"
						return nil
					}
				}
			}
		}

		// Fallback: use DescribeRoleZoneInfo → IsOpenBandWidthService
		// This works when DescribeIntranetAttribute doesn't expose the burst field
		zoneParams := &alicloudOpenapiClient.Params{
			Action:      tea.String("DescribeRoleZoneInfo"),
			Version:     tea.String("2015-01-01"),
			Protocol:    tea.String("HTTPS"),
			Pathname:    tea.String("/"),
			Method:      tea.String("POST"),
			AuthType:    tea.String("AK"),
			BodyType:    tea.String("json"),
			ReqBodyType: tea.String("json"),
			Style:       tea.String("RPC"),
		}
		zoneQueries := map[string]any{}
		zoneQueries["InstanceId"] = tea.String(instanceId)
		zoneReq := &alicloudOpenapiClient.OpenApiRequest{
			Query: openapiutil.Query(zoneQueries),
		}
		zoneResult, err := r.client.CallApi(zoneParams, zoneReq, runtime)
		if err != nil {
			return backoff.Permanent(err)
		}
		if zoneResult != nil {
			if body, ok := zoneResult["body"].(map[string]any); ok {
				if nodeContainer, ok := body["Node"].(map[string]any); ok {
					if nodes, ok := nodeContainer["NodeInfo"].([]any); ok && len(nodes) > 0 {
						if firstNode, ok := nodes[0].(map[string]any); ok {
							if val, ok := firstNode["IsOpenBandWidthService"]; ok {
								if b, ok := val.(bool); ok {
									burstEnabled = b
								}
							}
						}
					}
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
		params := &alicloudOpenapiClient.Params{
			Action:      tea.String("EnableAdditionalBandwidth"),
			Version:     tea.String("2015-01-01"),
			Protocol:    tea.String("HTTPS"),
			Pathname:    tea.String("/"),
			Method:      tea.String("POST"),
			AuthType:    tea.String("AK"),
			BodyType:    tea.String("json"),
			ReqBodyType: tea.String("json"),
			Style:       tea.String("RPC"),
		}
		queries := map[string]any{}
		queries["InstanceId"] = tea.String(instanceId)
		queries["BandWidthBurst"] = tea.String(burstStr)
		queries["ChargeType"] = tea.String("PostPaid")
		openapiReq := &alicloudOpenapiClient.OpenApiRequest{
			Query: openapiutil.Query(queries),
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
	reconnectBackoff.MaxElapsedTime = 5 * time.Minute
	err := backoff.Retry(enableBurst, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to set bandwidth burst for instance %s: %w", instanceId, err)
	}

	// Wait for instance to return to Normal status before returning
	// so subsequent operations don't hit InstanceStatus.NotSupport
	if waitErr := r.waitForInstanceNormal(instanceId, 5*time.Minute); waitErr != nil {
		return fmt.Errorf("burst set but instance %s did not return to Normal: %w", instanceId, waitErr)
	}
	return nil
}

// waitForInstanceNormal polls DescribeInstances until status == "Normal" or timeout
func (r *kvstoreInstanceBandwidthBurstResource) waitForInstanceNormal(instanceId string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Second

	for time.Now().Before(deadline) {
		runtime := &util.RuntimeOptions{}
		params := &alicloudOpenapiClient.Params{
			Action:      tea.String("DescribeInstances"),
			Version:     tea.String("2015-01-01"),
			Protocol:    tea.String("HTTPS"),
			Pathname:    tea.String("/"),
			Method:      tea.String("POST"),
			AuthType:    tea.String("AK"),
			BodyType:    tea.String("json"),
			ReqBodyType: tea.String("json"),
			Style:       tea.String("RPC"),
		}
		queries := map[string]any{}
		queries["RegionId"] = tea.String(r.region)
		queries["InstanceIds"] = tea.String(instanceId)
		openapiReq := &alicloudOpenapiClient.OpenApiRequest{
			Query: openapiutil.Query(queries),
		}
		result, err := r.client.CallApi(params, openapiReq, runtime)
		if err == nil && result != nil {
			if body, ok := result["body"].(map[string]any); ok {
				if insts, ok := body["Instances"].(map[string]any); ok {
					if kvInsts, ok := insts["KVStoreInstance"].([]any); ok && len(kvInsts) > 0 {
						if inst, ok := kvInsts[0].(map[string]any); ok {
							if status, ok := inst["InstanceStatus"].(string); ok && status == "Normal" {
								return nil
							}
						}
					}
				}
			}
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("timed out waiting for instance %s to reach Normal status", instanceId)
}
