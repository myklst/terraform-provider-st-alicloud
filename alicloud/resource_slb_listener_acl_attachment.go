package alicloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	openapiutil "github.com/alibabacloud-go/openapi-util/service"
	alicloudSlbClient "github.com/alibabacloud-go/slb-20140515/v4/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &slbListenerAclAttachmentResource{}
	_ resource.ResourceWithConfigure   = &slbListenerAclAttachmentResource{}
	_ resource.ResourceWithImportState = &slbListenerAclAttachmentResource{}
)

func NewSlbListenerAclAttachmentResource() resource.Resource {
	return &slbListenerAclAttachmentResource{}
}

type slbListenerAclAttachmentResource struct {
	client *alicloudSlbClient.Client
}

type slbListenerAclAttachmentModel struct {
	Id         types.String `tfsdk:"id"`
	ListenerId types.String `tfsdk:"listener_id"`
	AclIds     types.List   `tfsdk:"acl_ids"`
}

func (r *slbListenerAclAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_slb_listener_acl_attachment"
}

func (r *slbListenerAclAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attach ACL(s) to an SLB listener and enable access control with white list type.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Same as listener_id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"listener_id": schema.StringAttribute{
				Description: "The listener ID in the format load_balancer_id:protocol:port (e.g. lb-xxx:tcp:80).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"acl_ids": schema.ListAttribute{
				Description: "List of ACL IDs to attach to the listener.",
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

func (r *slbListenerAclAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).slbClient
}

// --- CRUD ---

func (r *slbListenerAclAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *slbListenerAclAttachmentModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.setAclConfig(ctx, plan.ListenerId.ValueString(), plan.AclIds, "on", "white")
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to attach ACL to SLB listener.",
			err.Error(),
		)
		return
	}

	state := &slbListenerAclAttachmentModel{
		Id:         plan.ListenerId,
		ListenerId: plan.ListenerId,
		AclIds:     plan.AclIds,
	}
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *slbListenerAclAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *slbListenerAclAttachmentModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	listenerId := state.ListenerId.ValueString()
	aclStatus, aclIds, err := r.readListenerAcl(listenerId)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read SLB listener ACL attribute.",
			err.Error(),
		)
		return
	}

	// If ACL is off, the attachment resource is gone —
	// even if AclId still has values, they're not active.
	if aclStatus != "on" {
		resp.State.RemoveResource(ctx)
		return
	}

	aclIdsValue, diags := types.ListValueFrom(ctx, types.StringType, aclIds)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state = &slbListenerAclAttachmentModel{
		Id:         types.StringValue(listenerId),
		ListenerId: types.StringValue(listenerId),
		AclIds:     aclIdsValue,
	}
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *slbListenerAclAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *slbListenerAclAttachmentModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.setAclConfig(ctx, plan.ListenerId.ValueString(), plan.AclIds, "on", "white")
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to update SLB listener ACL attachment.",
			err.Error(),
		)
		return
	}

	state := &slbListenerAclAttachmentModel{
		Id:         plan.ListenerId,
		ListenerId: plan.ListenerId,
		AclIds:     plan.AclIds,
	}
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
}

func (r *slbListenerAclAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *slbListenerAclAttachmentModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.deleteAclConfig(state.ListenerId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to disable SLB listener access control.",
			err.Error(),
		)
	}
}

func (r *slbListenerAclAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("listener_id"), req, resp)
}

// --- RPC helper ---

// callSlbRpc dispatches an SLB RPC call via CallApi with dynamic action name.
// This avoids the 4-protocol switch duplication in generated typed SDK methods.
func (r *slbListenerAclAttachmentResource) callSlbRpc(action string, params map[string]string) (map[string]any, error) {
	apiParams := &openapi.Params{
		Action:      tea.String(action),
		Version:     tea.String("2014-05-15"),
		Protocol:    tea.String("HTTPS"),
		Method:      tea.String("POST"),
		AuthType:    tea.String("AK"),
		Style:       tea.String("RPC"),
		Pathname:    tea.String("/"),
		ReqBodyType: tea.String("formData"),
		BodyType:    tea.String("json"),
	}

	queries := make(map[string]any, len(params))
	for k, v := range params {
		queries[k] = tea.String(v)
	}

	request := &openapi.OpenApiRequest{
		Query: openapiutil.Query(queries),
	}

	runtime := &util.RuntimeOptions{}
	resp, err := r.client.CallApi(apiParams, request, runtime)
	if err != nil {
		return nil, err
	}

	if body, ok := resp["body"].(map[string]any); ok {
		return body, nil
	}
	return resp, nil
}

// --- Helpers ---

// readListenerAcl reads the ACL configuration from the listener attribute API.
// Returns (aclStatus, aclIds, error). Retries on transient errors.
func (r *slbListenerAclAttachmentResource) readListenerAcl(listenerId string) (string, []string, error) {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return "", nil, err
	}

	action := fmt.Sprintf("DescribeLoadBalancer%sListenerAttribute", strings.ToUpper(protocol))
	var aclStatus, aclIdStr string

	readFn := func() error {
		body, apiErr := r.callSlbRpc(action, map[string]string{
			"LoadBalancerId": loadBalancerId,
			"ListenerPort":   fmt.Sprintf("%d", listenerPort),
		})
		if apiErr != nil {
			if _t, ok := apiErr.(*tea.SDKError); ok && isAbleToRetry(*_t.Code) {
				return apiErr
			}
			return backoff.Permanent(apiErr)
		}
		aclStatus, _ = body["AclStatus"].(string)
		aclIdStr, _ = body["AclId"].(string)
		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(readFn, bo)
	if err != nil {
		return "", nil, err
	}

	var aclIds []string
	if aclIdStr != "" {
		aclIds = strings.Split(aclIdStr, ",")
	}
	return aclStatus, aclIds, nil
}

// setAclConfig enables ACL on the listener with the given ACL IDs, type, and status.
// Retries on OperationFailed.ListenerStatusNotSupport since that's a transient error.
func (r *slbListenerAclAttachmentResource) setAclConfig(ctx context.Context, listenerId string, aclIdsList types.List, aclStatus string, aclType string) error {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return err
	}

	var aclIdStrs []string
	diags := aclIdsList.ElementsAs(ctx, &aclIdStrs, false)
	if diags.HasError() {
		return fmt.Errorf("failed to convert acl_ids list: %v", diags)
	}
	aclIds := strings.Join(aclIdStrs, ",")

	action := fmt.Sprintf("SetLoadBalancer%sListenerAttribute", strings.ToUpper(protocol))

	setAcl := func() error {
		_, apiErr := r.callSlbRpc(action, map[string]string{
			"LoadBalancerId": loadBalancerId,
			"ListenerPort":   fmt.Sprintf("%d", listenerPort),
			"AclStatus":      aclStatus,
			"AclType":        aclType,
			"AclId":          aclIds,
		})
		if apiErr != nil {
			if isRetryableOrStatusError(apiErr) {
				return apiErr
			}
			return backoff.Permanent(apiErr)
		}
		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(setAcl, bo)
	if err != nil {
		return fmt.Errorf("failed to set ACL on listener: %w", err)
	}

	return nil
}

// deleteAclConfig disables ACL on the listener by setting AclStatus="off".
// Only sends AclStatus — does NOT send AclType or AclId to avoid corrupting
// the listener config. Retries on transient status errors.
// If the listener is already gone, treat as success.
func (r *slbListenerAclAttachmentResource) deleteAclConfig(listenerId string) error {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return err
	}

	action := fmt.Sprintf("SetLoadBalancer%sListenerAttribute", strings.ToUpper(protocol))

	setAcl := func() error {
		_, apiErr := r.callSlbRpc(action, map[string]string{
			"LoadBalancerId": loadBalancerId,
			"ListenerPort":   fmt.Sprintf("%d", listenerPort),
			"AclStatus":      "off",
		})
		if apiErr != nil {
			if sdkErr, ok := apiErr.(*tea.SDKError); ok && sdkErr.Code != nil {
				code := *sdkErr.Code
				if code == "InvalidListener" || code == "NoSuchListener" ||
					code == "InvalidLoadBalancerId.NotFound" || code == "ResourceNotFound" {
					return nil
				}
			}
			if isRetryableOrStatusError(apiErr) {
				return apiErr
			}
			return backoff.Permanent(apiErr)
		}
		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(setAcl, bo)
	if err != nil {
		return fmt.Errorf("failed to disable ACL on listener: %w", err)
	}

	return nil
}

// parseListenerId parses "lb-xxx:protocol:port" into (loadBalancerId, protocol, listenerPort).
func parseListenerId(listenerId string) (loadBalancerId string, protocol string, listenerPort int64, err error) {
	parts := strings.Split(listenerId, ":")
	if len(parts) != 3 {
		err = fmt.Errorf("invalid listener_id format: %q, expected load_balancer_id:protocol:port", listenerId)
		return
	}
	loadBalancerId = parts[0]
	protocol = parts[1]
	_, err = fmt.Sscanf(parts[2], "%d", &listenerPort)
	if err != nil {
		err = fmt.Errorf("invalid port in listener_id %q: %w", listenerId, err)
	}
	return
}

// isRetryableOrStatusError returns true if the error is retryable (from the global list)
// or a listener status error (OperationFailed.ListenerStatusNotSupport), which is transient.
func isRetryableOrStatusError(err error) bool {
	sdkErr, ok := err.(*tea.SDKError)
	if !ok || sdkErr.Code == nil {
		return false
	}
	if strings.ToLower(*sdkErr.Code) == "operationfailed.listenerstatusnotsupport" {
		return true
	}
	return isAbleToRetry(*sdkErr.Code)
}
