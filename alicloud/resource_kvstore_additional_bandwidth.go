package alicloud

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudOpenapiClient "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	openapiutil "github.com/alibabacloud-go/openapi-util/service"
)

var (
	_ resource.Resource                = &kvstoreAdditionalBandwidthResource{}
	_ resource.ResourceWithConfigure   = &kvstoreAdditionalBandwidthResource{}
	_ resource.ResourceWithImportState = &kvstoreAdditionalBandwidthResource{}
)

func NewKvstoreAdditionalBandwidthResource() resource.Resource {
	return &kvstoreAdditionalBandwidthResource{}
}

type kvstoreAdditionalBandwidthResource struct {
	client *alicloudOpenapiClient.Client
}

type kvstoreAdditionalBandwidthModel struct {
	Id             types.String `tfsdk:"id"`
	InstanceId     types.String `tfsdk:"instance_id"`
	NodeId         types.String `tfsdk:"node_id"`
	Bandwidth      types.Int64  `tfsdk:"bandwidth"`
	BandwidthBurst types.Bool   `tfsdk:"bandwidth_burst"`
}

func (r *kvstoreAdditionalBandwidthResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kvstore_additional_bandwidth"
}

func (r *kvstoreAdditionalBandwidthResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages additional bandwidth and elastic burst for an Alibaba Cloud Redis (R-Kvstore) instance. " +
			"Omit node_id for instance-level burst (applies to all nodes). " +
			"Set node_id to a specific shard for per-shard additional bandwidth.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The resource ID. Format: instance_id (instance-level) or instance_id:node_id (per-shard).",
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
				Description: "The shard (node) ID for per-shard bandwidth, in InsName format (e.g. \"r-xxx-db-0\"). " +
					"Omit or set to \"All\" for instance-level burst (applies to all nodes). " +
					"Use DescribeRoleZoneInfo or DescribeLogicInstanceTopology to list available node IDs.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("All"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bandwidth": schema.Int64Attribute{
				Description: "Additional bandwidth in MB/s for a specific shard. " +
					"Set to 0 (default) for instance-level burst only. " +
					"Must be a positive integer for per-shard additional bandwidth.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(0),
			},
			"bandwidth_burst": schema.BoolAttribute{
				Description: "Whether to enable bandwidth burst. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
		},
	}
}

func (r *kvstoreAdditionalBandwidthResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).kvstoreRawClient
}

// --- helpers ---

func isInstanceLevel(nodeId string) bool {
	return nodeId == "" || nodeId == "All"
}

func makeId(instanceId, nodeId string) string {
	if isInstanceLevel(nodeId) {
		return instanceId
	}
	return fmt.Sprintf("%s:%s", instanceId, nodeId)
}

// --- CRUD ---

func (r *kvstoreAdditionalBandwidthResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *kvstoreAdditionalBandwidthModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	instanceId := plan.InstanceId.ValueString()
	nodeId := plan.NodeId.ValueString()
	bandwidth := plan.Bandwidth.ValueInt64()
	burst := plan.BandwidthBurst.ValueBool()

	err := r.enableAdditionalBandwidth(instanceId, nodeId, bandwidth, burst)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to set Redis additional bandwidth.",
			err.Error(),
		)
		return
	}

	// Read back and verify the apply actually took effect
	if verifyErr := r.verifyApplied(instanceId, nodeId, bandwidth, burst); verifyErr != nil {
		resp.Diagnostics.AddError(
			"[VERIFY ERROR] Apply succeeded but read-back verification failed.",
			verifyErr.Error(),
		)
		return
	}

	state := &kvstoreAdditionalBandwidthModel{
		Id:             types.StringValue(makeId(instanceId, nodeId)),
		InstanceId:     plan.InstanceId,
		NodeId:         plan.NodeId,
		Bandwidth:      plan.Bandwidth,
		BandwidthBurst: plan.BandwidthBurst,
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreAdditionalBandwidthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *kvstoreAdditionalBandwidthModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	instanceId := state.InstanceId.ValueString()
	nodeId := state.NodeId.ValueString()

	currentBw, defaultBw, _, err := r.readBandwidth(instanceId, nodeId)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read Redis bandwidth.",
			err.Error(),
		)
		return
	}

	state.Id = types.StringValue(makeId(instanceId, nodeId))

	if isInstanceLevel(nodeId) {
		state.Bandwidth = types.Int64Value(0)
	} else {
		// During import, bandwidth is null — compute from API
		if state.Bandwidth.IsNull() {
			additional := currentBw - defaultBw
			if additional < 0 {
				additional = 0
			}
			state.Bandwidth = types.Int64Value(additional)
		}
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreAdditionalBandwidthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *kvstoreAdditionalBandwidthModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	instanceId := plan.InstanceId.ValueString()
	nodeId := plan.NodeId.ValueString()
	bandwidth := plan.Bandwidth.ValueInt64()
	burst := plan.BandwidthBurst.ValueBool()

	err := r.enableAdditionalBandwidth(instanceId, nodeId, bandwidth, burst)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to update Redis additional bandwidth.",
			err.Error(),
		)
		return
	}

	// Read back and verify the update actually took effect
	if verifyErr := r.verifyApplied(instanceId, nodeId, bandwidth, burst); verifyErr != nil {
		resp.Diagnostics.AddError(
			"[VERIFY ERROR] Update succeeded but read-back verification failed.",
			verifyErr.Error(),
		)
		return
	}

	state := &kvstoreAdditionalBandwidthModel{
		Id:             types.StringValue(makeId(instanceId, nodeId)),
		InstanceId:     plan.InstanceId,
		NodeId:         plan.NodeId,
		Bandwidth:      plan.Bandwidth,
		BandwidthBurst: plan.BandwidthBurst,
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *kvstoreAdditionalBandwidthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *kvstoreAdditionalBandwidthModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	instanceId := state.InstanceId.ValueString()
	nodeId := state.NodeId.ValueString()

	// Reset: bandwidth=0, burst=false
	err := r.enableAdditionalBandwidth(instanceId, nodeId, 0, false)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to reset Redis additional bandwidth.",
			err.Error(),
		)
		return
	}
}

func (r *kvstoreAdditionalBandwidthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Format: instance_id (instance-level) or instance_id:node_id (per-shard)
	parts := strings.SplitN(req.ID, ":", 2)

	if len(parts) == 1 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), types.StringValue(parts[0]))...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), types.StringValue("All"))...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(parts[0]))...)
	} else {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), types.StringValue(parts[0]))...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), types.StringValue(parts[1]))...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	}
}

// --- API helpers ---

// enableAdditionalBandwidth calls EnableAdditionalBandwidth API.
// For instance-level (nodeId="All"): toggles burst, bandwidth=0.
// For per-shard (nodeId=specific): sets additional bandwidth, burst=true.
func (r *kvstoreAdditionalBandwidthResource) enableAdditionalBandwidth(instanceId, nodeId string, bandwidth int64, burst bool) error {
	burstStr := "false"
	if burst {
		burstStr = "true"
	}
	bwStr := strconv.FormatInt(bandwidth, 10)

	callApi := func() error {
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
		queries["NodeId"] = tea.String(nodeId)
		queries["Bandwidth"] = tea.String(bwStr)
		queries["BandWidthBurst"] = tea.String(burstStr)
		queries["ChargeType"] = tea.String("PostPaid")
		queries["AutoPay"] = tea.String("true")
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
	err := backoff.Retry(callApi, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to set additional bandwidth for instance %s (node %s): %w", instanceId, nodeId, err)
	}

	if waitErr := r.waitForInstanceNormal(instanceId, 5*time.Minute); waitErr != nil {
		return fmt.Errorf("bandwidth set but instance %s did not return to Normal: %w", instanceId, waitErr)
	}
	return nil
}

// verifyApplied reads back the instance state after an apply and confirms the
// requested bandwidth/burst values actually took effect. This catches cases
// where the API returns success but the change was silently ignored.
//
// Per-shard: CurrentBandWidth - DefaultBandWidth must equal requested bandwidth.
// Instance-level (burst): IntranetBandWidthBurst > 0 must match bandwidth_burst=true.
func (r *kvstoreAdditionalBandwidthResource) verifyApplied(instanceId, nodeId string, bandwidth int64, burst bool) error {
	if isInstanceLevel(nodeId) {
		// Instance-level: verify burst state
		burstBw, err := r.readBurstValue(instanceId)
		if err != nil {
			return fmt.Errorf("failed to read burst status for verification: %w", err)
		}
		if burst && burstBw <= 0 {
			return fmt.Errorf("bandwidth_burst=true but instance %s burst is not enabled (IntranetBandWidthBurst=0)", instanceId)
		}
		if !burst && burstBw > 0 {
			return fmt.Errorf("bandwidth_burst=false but instance %s burst is still enabled (IntranetBandWidthBurst=%d)", instanceId, burstBw)
		}
		return nil
	}

	// Per-shard: verify bandwidth value
	currentBw, defaultBw, _, err := r.readNodeBandwidth(instanceId, nodeId)
	if err != nil {
		return fmt.Errorf("failed to read node bandwidth for verification: %w", err)
	}
	actual := currentBw - defaultBw
	if actual < 0 {
		actual = 0
	}
	if actual != bandwidth {
		return fmt.Errorf("bandwidth mismatch for node %s: requested %d MB/s, got %d MB/s (CurrentBandWidth=%d, DefaultBandWidth=%d)",
			nodeId, bandwidth, actual, currentBw, defaultBw)
	}
	return nil
}

// readBurstValue reads IntranetBandWidthBurst from DescribeIntranetAttribute.
// Returns the burst cap in MB/s. 0 = burst disabled.
func (r *kvstoreAdditionalBandwidthResource) readBurstValue(instanceId string) (int64, error) {
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
		return 0, err
	}
	if result == nil {
		return 0, fmt.Errorf("no response from DescribeIntranetAttribute for instance %s", instanceId)
	}
	body, ok := result["body"].(map[string]any)
	if !ok {
		return 0, fmt.Errorf("invalid response body for instance %s", instanceId)
	}
	return toInt64(body["IntranetBandWidthBurst"]), nil
}

// readBandwidth returns currentBw, defaultBw, isBwOpen for the given node.
// Instance-level: returns 0, 0, false (no per-shard data).
// Per-shard: reads from DescribeRoleZoneInfo.
func (r *kvstoreAdditionalBandwidthResource) readBandwidth(instanceId, nodeId string) (currentBw, defaultBw int64, isBwOpen bool, err error) {
	if isInstanceLevel(nodeId) {
		return 0, 0, false, nil
	}
	return r.readNodeBandwidth(instanceId, nodeId)
}

// readNodeBandwidth reads per-shard bandwidth from DescribeRoleZoneInfo.
// Matches by InsName (e.g. "r-xxx-db-0") which is what EnableAdditionalBandwidth expects.
func (r *kvstoreAdditionalBandwidthResource) readNodeBandwidth(instanceId, nodeId string) (currentBw, defaultBw int64, isBwOpen bool, err error) {
	readBwFn := func() error {
		runtime := &util.RuntimeOptions{}
		params := &alicloudOpenapiClient.Params{
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

		if result == nil {
			return fmt.Errorf("no response from DescribeRoleZoneInfo for instance %s", instanceId)
		}

		body, ok := result["body"].(map[string]any)
		if !ok {
			return fmt.Errorf("invalid response body for instance %s", instanceId)
		}

		nodeContainer, ok := body["Node"].(map[string]any)
		if !ok {
			return fmt.Errorf("no Node in response for instance %s", instanceId)
		}

		nodes, ok := nodeContainer["NodeInfo"].([]any)
		if !ok || len(nodes) == 0 {
			return fmt.Errorf("no NodeInfo in response for instance %s", instanceId)
		}

		// Match by InsName (e.g. "r-xxx-db-0") — this is the format EnableAdditionalBandwidth expects.
		// DescribeRoleZoneInfo returns both MASTER and SLAVE for each shard; we take the first match
		// (usually MASTER) since bandwidth is identical for both.
		for _, n := range nodes {
			node, ok := n.(map[string]any)
			if !ok {
				continue
			}
			insName, ok := node["InsName"].(string)
			if !ok {
				continue
			}
			if insName == nodeId {
				currentBw = toInt64(node["CurrentBandWidth"])
				defaultBw = toInt64(node["DefaultBandWidth"])
				if v, ok := node["IsOpenBandWidthService"].(bool); ok {
					isBwOpen = v
				}
				return nil
			}
		}

		return backoff.Permanent(fmt.Errorf("node %s not found in instance %s (matched by InsName)", nodeId, instanceId))
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 5 * time.Minute
	err = backoff.Retry(readBwFn, reconnectBackoff)
	if err != nil {
		return 0, 0, false, fmt.Errorf("failed to read bandwidth for shard %s: %w", nodeId, err)
	}
	return currentBw, defaultBw, isBwOpen, nil
}

// waitForInstanceNormal polls DescribeInstances until status == "Normal" or timeout
func (r *kvstoreAdditionalBandwidthResource) waitForInstanceNormal(instanceId string, timeout time.Duration) error {
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

// toInt64 converts a value from JSON unmarshalling to int64.
func toInt64(v any) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	case json.Number:
		n, _ := val.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	}
	return 0
}
