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

	// When burst is enabled, apply it first so the burst cap (IntranetBandWidthBurst)
	// is populated. This lets validateBandwidth read the real max limit.
	if burst {
		if err := r.enableAdditionalBandwidth(instanceId, nodeId, 0, true); err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to enable bandwidth burst.",
				err.Error(),
			)
			return
		}
	}

	if err := r.validateBandwidth(instanceId, nodeId, bandwidth); err != nil {
		resp.Diagnostics.AddError("[VALIDATION ERROR]", err.Error())
		return
	}

	err := r.enableAdditionalBandwidth(instanceId, nodeId, bandwidth, burst)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to set Redis additional bandwidth.",
			err.Error(),
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

	currentBw, defaultBw, _, _, err := r.readBandwidth(instanceId, nodeId)
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

	// When burst is enabled, apply it first so the burst cap (IntranetBandWidthBurst)
	// is populated. This lets validateBandwidth read the real max limit.
	if burst {
		if err := r.enableAdditionalBandwidth(instanceId, nodeId, 0, true); err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to enable bandwidth burst.",
				err.Error(),
			)
			return
		}
	}

	if err := r.validateBandwidth(instanceId, nodeId, bandwidth); err != nil {
		resp.Diagnostics.AddError("[VALIDATION ERROR]", err.Error())
		return
	}

	err := r.enableAdditionalBandwidth(instanceId, nodeId, bandwidth, burst)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to update Redis additional bandwidth.",
			err.Error(),
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

// readBandwidth returns currentBw, defaultBw, burstBw, isBwOpen for the given node.
// Instance-level: reads burst bandwidth via DescribeIntranetAttribute (currentBw=0, defaultBw=0).
// Per-shard: reads from DescribeRoleZoneInfo (burstBw=0, isBwOpen from node).
func (r *kvstoreAdditionalBandwidthResource) readBandwidth(instanceId, nodeId string) (currentBw, defaultBw, burstBw int64, isBwOpen bool, err error) {
	if isInstanceLevel(nodeId) {
		burstBw, err = r.readBurst(instanceId)
		if err != nil {
			return 0, 0, 0, false, err
		}
		return 0, 0, burstBw, burstBw > 0, nil
	}
	currentBw, defaultBw, isBwOpen, err = r.readNodeBandwidth(instanceId, nodeId)
	return currentBw, defaultBw, 0, isBwOpen, err
}

// validateBandwidth checks that the requested additional bandwidth does not exceed
// the instance's burst cap. Only validates when burst is enabled (IntranetBandWidthBurst > 0).
// When burst is disabled (0), we skip validation — the API will reject invalid values.
//
// Per-shard: bandwidth <= IntranetBandWidthBurst - DefaultBandWidth
// Instance-level (All): bandwidth <= (IntranetBandWidthBurst - DefaultBandWidth) * shardCount
func (r *kvstoreAdditionalBandwidthResource) validateBandwidth(instanceId, nodeId string, bandwidth int64) error {
	if bandwidth <= 0 {
		return nil
	}

	burstBw, err := r.readBurst(instanceId)
	if err != nil {
		return fmt.Errorf("failed to read burst bandwidth for validation: %w", err)
	}

	// Burst disabled — can't enforce limit, let API decide
	if burstBw <= 0 {
		return nil
	}

	defaultBw, shardCount, err := r.readDefaultBandwidthAndShardCount(instanceId)
	if err != nil {
		return fmt.Errorf("failed to read instance info for validation: %w", err)
	}

	maxPerShard := burstBw - defaultBw
	if maxPerShard < 0 {
		maxPerShard = 0
	}

	if isInstanceLevel(nodeId) {
		maxTotal := maxPerShard * shardCount
		if bandwidth > maxTotal {
			return fmt.Errorf("bandwidth %d MB/s exceeds instance limit %d MB/s (per-shard max %d × %d shards). IntranetBandWidthBurst=%d, DefaultBandWidth=%d",
				bandwidth, maxTotal, maxPerShard, shardCount, burstBw, defaultBw)
		}
	} else {
		if bandwidth > maxPerShard {
			return fmt.Errorf("bandwidth %d MB/s exceeds per-shard limit %d MB/s (IntranetBandWidthBurst=%d - DefaultBandWidth=%d)",
				bandwidth, maxPerShard, burstBw, defaultBw)
		}
	}
	return nil
}

// readDefaultBandwidthAndShardCount reads the per-shard default bandwidth and shard count
// from DescribeRoleZoneInfo. Returns the first node's DefaultBandWidth and the count of
// unique InsNames (shards).
func (r *kvstoreAdditionalBandwidthResource) readDefaultBandwidthAndShardCount(instanceId string) (defaultBw int64, shardCount int64, err error) {
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
	result, callErr := r.client.CallApi(params, openapiReq, runtime)
	if callErr != nil {
		return 0, 0, callErr
	}

	if result == nil {
		return 0, 0, fmt.Errorf("no response from DescribeRoleZoneInfo for instance %s", instanceId)
	}

	body, ok := result["body"].(map[string]any)
	if !ok {
		return 0, 0, fmt.Errorf("invalid response body for instance %s", instanceId)
	}

	nodeContainer, ok := body["Node"].(map[string]any)
	if !ok {
		return 0, 0, fmt.Errorf("no Node in response for instance %s", instanceId)
	}

	nodes, ok := nodeContainer["NodeInfo"].([]any)
	if !ok || len(nodes) == 0 {
		return 0, 0, fmt.Errorf("no NodeInfo in response for instance %s", instanceId)
	}

	// Count unique InsNames (shards). MASTER+SLAVE share the same InsName.
	seen := map[string]bool{}
	for _, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			continue
		}
		if insName, ok := node["InsName"].(string); ok {
			seen[insName] = true
		}
		// Use first node's DefaultBandWidth
		if defaultBw == 0 {
			defaultBw = toInt64(node["DefaultBandWidth"])
		}
	}
	shardCount = int64(len(seen))
	return defaultBw, shardCount, nil
}

// readBurst reads instance-level burst bandwidth value from DescribeIntranetAttribute.
// Returns the IntranetBandWidthBurst value (integer MB/s). 0 = burst disabled.
// Fallback: DescribeRoleZoneInfo → first node IsOpenBandWidthService (returns 1 if true, 0 if false)
func (r *kvstoreAdditionalBandwidthResource) readBurst(instanceId string) (int64, error) {
	var burstBw int64

	readBurstFn := func() error {
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

		if result != nil {
			if body, ok := result["body"].(map[string]any); ok {
				if burstVal, ok := body["IntranetBandWidthBurst"]; ok {
					// IntranetBandWidthBurst is an integer (burst bandwidth in MB/s),
					// NOT a boolean. > 0 means burst is enabled.
					burstBw = toInt64(burstVal)
					return nil
				}
			}
		}

		// Fallback: DescribeRoleZoneInfo → first node IsOpenBandWidthService
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
								if b, ok := val.(bool); ok && b {
									burstBw = 1
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
	err := backoff.Retry(readBurstFn, reconnectBackoff)
	if err != nil {
		return 0, fmt.Errorf("failed to read burst status for instance %s: %w", instanceId, err)
	}
	return burstBw, nil
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
