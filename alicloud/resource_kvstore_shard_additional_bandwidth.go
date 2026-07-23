package alicloud

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	util "github.com/alibabacloud-go/tea-utils/service"
	utilV2 "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudKvstoreClient "github.com/alibabacloud-go/r-kvstore-20150101/client"
	alicloudOpenapiClient "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	openapiutil "github.com/alibabacloud-go/openapi-util/service"
)

var (
	_ resource.Resource                = &kvstoreShardAdditionalBandwidthResource{}
	_ resource.ResourceWithConfigure   = &kvstoreShardAdditionalBandwidthResource{}
	_ resource.ResourceWithImportState = &kvstoreShardAdditionalBandwidthResource{}
)

func NewKvstoreShardAdditionalBandwidthResource() resource.Resource {
	return &kvstoreShardAdditionalBandwidthResource{}
}

type kvstoreShardAdditionalBandwidthResource struct {
	client    *alicloudKvstoreClient.Client
	rawClient *alicloudOpenapiClient.Client
}

type kvstoreShardAdditionalBandwidthModel struct {
	Id           types.String `tfsdk:"id"`
	InstanceId   types.String `tfsdk:"instance_id"`
	NodeId       types.String `tfsdk:"node_id"`
	Bandwidth    types.Int64  `tfsdk:"bandwidth"`
	CurrentBw    types.Int64  `tfsdk:"current_bandwidth"`
	IsBurstOpen  types.Bool   `tfsdk:"is_burst_open"`
}

func (r *kvstoreShardAdditionalBandwidthResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kvstore_shard_additional_bandwidth"
}

func (r *kvstoreShardAdditionalBandwidthResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Configures additional bandwidth for a specific shard (node) of an Alibaba Cloud Redis (R-Kvstore) instance. " +
			"Use this resource when specific shards need different bandwidth than the instance default. " +
			"Shards that don't need extra bandwidth are managed by the official alicloud_kvstore_instance bandwidth field.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The resource ID in the format instance_id:node_id.",
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
			"node_id": schema.StringAttribute{
				Description: "The shard (node) ID to configure additional bandwidth for. " +
					"Use DescribeRoleZoneInfo to list available node IDs.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bandwidth": schema.Int64Attribute{
				Description: "Additional bandwidth in MB/s for this shard. Must be a positive integer. " +
					"Set to 0 to remove additional bandwidth (resets to instance default).",
				Required: true,
			},
			"current_bandwidth": schema.Int64Attribute{
				Description: "The current total bandwidth of this shard (read-only).",
				Computed:    true,
			},
			"is_burst_open": schema.BoolAttribute{
				Description: "Whether bandwidth burst service is open for this shard (read-only).",
				Computed:    true,
			},
		},
	}
}

func (r *kvstoreShardAdditionalBandwidthResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).kvstoreClient
	r.rawClient = req.ProviderData.(alicloudClients).kvstoreRawClient
}

func (r *kvstoreShardAdditionalBandwidthResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *kvstoreShardAdditionalBandwidthModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	instanceId := plan.InstanceId.ValueString()
	nodeId := plan.NodeId.ValueString()
	bandwidth := plan.Bandwidth.ValueInt64()

	if bandwidth > 0 {
		err := r.enableAdditionalBandwidth(instanceId, nodeId, bandwidth)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to enable additional bandwidth for shard.",
				err.Error(),
			)
			return
		}
	}

	// Read back current state
	currentBw, isBurstOpen, err := r.readNodeBandwidth(instanceId, nodeId)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read shard bandwidth after create.",
			err.Error(),
		)
		return
	}

	state := &kvstoreShardAdditionalBandwidthModel{
		Id:          types.StringValue(fmt.Sprintf("%s:%s", instanceId, nodeId)),
		InstanceId:  plan.InstanceId,
		NodeId:      plan.NodeId,
		Bandwidth:   plan.Bandwidth,
		CurrentBw:   types.Int64Value(currentBw),
		IsBurstOpen: types.BoolValue(isBurstOpen),
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreShardAdditionalBandwidthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *kvstoreShardAdditionalBandwidthModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	instanceId := state.InstanceId.ValueString()
	nodeId := state.NodeId.ValueString()

	currentBw, isBurstOpen, err := r.readNodeBandwidth(instanceId, nodeId)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read shard bandwidth.",
			err.Error(),
		)
		return
	}

	// If the node's bandwidth has been reset to default (no additional BW),
	// and the user hasn't set bandwidth=0, the resource is gone
	if currentBw == 0 && state.Bandwidth.ValueInt64() > 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Id = types.StringValue(fmt.Sprintf("%s:%s", instanceId, nodeId))
	state.CurrentBw = types.Int64Value(currentBw)
	state.IsBurstOpen = types.BoolValue(isBurstOpen)

	// During import, bandwidth may be unknown (null) — populate from current bandwidth
	if state.Bandwidth.IsNull() {
		state.Bandwidth = types.Int64Value(currentBw)
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreShardAdditionalBandwidthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *kvstoreShardAdditionalBandwidthModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	instanceId := plan.InstanceId.ValueString()
	nodeId := plan.NodeId.ValueString()
	bandwidth := plan.Bandwidth.ValueInt64()

	if bandwidth > 0 {
		err := r.enableAdditionalBandwidth(instanceId, nodeId, bandwidth)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to update additional bandwidth for shard.",
				err.Error(),
			)
			return
		}
	} else {
		// bandwidth=0 means reset to default
		err := r.resetBandwidth(instanceId, nodeId)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to reset shard bandwidth to default.",
				err.Error(),
			)
			return
		}
	}

	// Read back current state
	currentBw, isBurstOpen, err := r.readNodeBandwidth(instanceId, nodeId)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read shard bandwidth after update.",
			err.Error(),
		)
		return
	}

	state := &kvstoreShardAdditionalBandwidthModel{
		Id:          types.StringValue(fmt.Sprintf("%s:%s", instanceId, nodeId)),
		InstanceId:  plan.InstanceId,
		NodeId:      plan.NodeId,
		Bandwidth:   plan.Bandwidth,
		CurrentBw:   types.Int64Value(currentBw),
		IsBurstOpen: types.BoolValue(isBurstOpen),
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreShardAdditionalBandwidthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *kvstoreShardAdditionalBandwidthModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.resetBandwidth(state.InstanceId.ValueString(), state.NodeId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to reset shard bandwidth on delete.",
			err.Error(),
		)
		return
	}
}

func (r *kvstoreShardAdditionalBandwidthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: instance_id:node_id
	// e.g. terraform import st-alicloud_kvstore_shard_additional_bandwidth.foo r-xxxxx:r-xxxxx-db-0
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"[IMPORT ERROR] Invalid import ID format.",
			"Expected format: instance_id:node_id (e.g. r-xxxxx:r-xxxxx-db-0). Got: "+req.ID,
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), types.StringValue(parts[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), types.StringValue(parts[1]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
}

// enableAdditionalBandwidth calls EnableAdditionalBandwidth API for a specific shard.
// Uses CallApi because the Go SDK v1.0.7 doesn't expose BandWidthBurst/ChargeType fields.
func (r *kvstoreShardAdditionalBandwidthResource) enableAdditionalBandwidth(instanceId, nodeId string, bandwidth int64) error {
	bwStr := strconv.FormatInt(bandwidth, 10)

	enableBw := func() error {
		runtime := &utilV2.RuntimeOptions{}
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
		queries["NodeId"] = tea.String(nodeId)
		queries["Bandwidth"] = tea.String(bwStr)
		queries["ChargeType"] = tea.String("PostPaid")
		openapiReq := &alicloudOpenapiClient.OpenApiRequest{
			Query: openapiutil.Query(queries),
		}
		_, err := r.rawClient.CallApi(params, openapiReq, runtime)
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
	err := backoff.Retry(enableBw, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to enable additional bandwidth for shard %s: %w", nodeId, err)
	}

	// Wait for instance to return to Normal before returning
	if waitErr := r.waitForInstanceNormal(instanceId, 5*time.Minute); waitErr != nil {
		return fmt.Errorf("bandwidth set but instance %s did not return to Normal: %w", instanceId, waitErr)
	}
	return nil
}

// resetBandwidth resets a shard's additional bandwidth via EnableAdditionalBandwidth with bandwidth=0.
// Note: ModifyIntranetAttribute doesn't work for all instance types — the API returns
// ModifyBandWidth.NotSupport and tells us to use EnableAdditionalBandwidth instead.
func (r *kvstoreShardAdditionalBandwidthResource) resetBandwidth(instanceId, nodeId string) error {
	resetBw := func() error {
		runtime := &utilV2.RuntimeOptions{}
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
		queries["NodeId"] = tea.String(nodeId)
		queries["Bandwidth"] = tea.String("0")
		queries["ChargeType"] = tea.String("PostPaid")
		openapiReq := &alicloudOpenapiClient.OpenApiRequest{
			Query: openapiutil.Query(queries),
		}
		_, err := r.rawClient.CallApi(params, openapiReq, runtime)
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
	err := backoff.Retry(resetBw, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to reset bandwidth for shard %s: %w", nodeId, err)
	}

	// Wait for instance to return to Normal before returning
	if waitErr := r.waitForInstanceNormal(instanceId, 5*time.Minute); waitErr != nil {
		return fmt.Errorf("bandwidth reset but instance %s did not return to Normal: %w", instanceId, waitErr)
	}
	return nil
}

// readNodeBandwidth reads the current bandwidth and burst status for a specific shard
// via DescribeRoleZoneInfo.
func (r *kvstoreShardAdditionalBandwidthResource) readNodeBandwidth(instanceId, nodeId string) (currentBw int64, isBurstOpen bool, err error) {
	readBw := func() error {
		runtime := &util.RuntimeOptions{}
		request := &alicloudKvstoreClient.DescribeRoleZoneInfoRequest{
			InstanceId: tea.String(instanceId),
		}
		response, err := r.client.DescribeRoleZoneInfoWithOptions(request, runtime)
		if err != nil {
			if t, ok := err.(*tea.SDKError); ok {
				if isAbleToRetry(*t.Code) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}

		if response.Body == nil || response.Body.Node == nil {
			return fmt.Errorf("no node info returned for instance %s", instanceId)
		}

		for _, node := range response.Body.Node.NodeInfo {
			if tea.StringValue(node.NodeId) == nodeId {
				currentBw = tea.Int64Value(node.CurrentBandWidth)
				isBurstOpen = tea.BoolValue(node.IsOpenBandWidthService)
				return nil
			}
		}

		return backoff.Permanent(fmt.Errorf("node %s not found in instance %s", nodeId, instanceId))
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 5 * time.Minute
	err = backoff.Retry(readBw, reconnectBackoff)
	if err != nil {
		return 0, false, fmt.Errorf("failed to read bandwidth for shard %s: %w", nodeId, err)
	}
	return currentBw, isBurstOpen, nil
}

// waitForInstanceNormal polls DescribeInstances until status == "Normal" or timeout
func (r *kvstoreShardAdditionalBandwidthResource) waitForInstanceNormal(instanceId string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Second

	for time.Now().Before(deadline) {
		runtime := &utilV2.RuntimeOptions{}
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
		queries["InstanceIds"] = tea.String(instanceId)
		openapiReq := &alicloudOpenapiClient.OpenApiRequest{
			Query: openapiutil.Query(queries),
		}
		result, err := r.rawClient.CallApi(params, openapiReq, runtime)
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
