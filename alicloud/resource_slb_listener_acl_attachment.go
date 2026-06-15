package alicloud

import (
	"context"
	"fmt"
	"strings"
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

	alicloudSlbClient "github.com/alibabacloud-go/slb-20140515/v4/client"
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

// describeListenerAcl calls the protocol-specific DescribeLoadBalancerListenerAttribute API.
// Returns (aclStatus, aclId, error). aclId is comma-separated if multiple.
func (r *slbListenerAclAttachmentResource) describeListenerAcl(loadBalancerId, protocol string, listenerPort int64) (string, string, error) {
	runtime := &util.RuntimeOptions{}

	switch strings.ToLower(protocol) {
	case "http":
		resp, err := r.client.DescribeLoadBalancerHTTPListenerAttributeWithOptions(
			&alicloudSlbClient.DescribeLoadBalancerHTTPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
			}, runtime)
		if err != nil {
			return "", "", err
		}
		return tea.StringValue(resp.Body.AclStatus), tea.StringValue(resp.Body.AclId), nil
	case "https":
		resp, err := r.client.DescribeLoadBalancerHTTPSListenerAttributeWithOptions(
			&alicloudSlbClient.DescribeLoadBalancerHTTPSListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
			}, runtime)
		if err != nil {
			return "", "", err
		}
		return tea.StringValue(resp.Body.AclStatus), tea.StringValue(resp.Body.AclId), nil
	case "tcp":
		resp, err := r.client.DescribeLoadBalancerTCPListenerAttributeWithOptions(
			&alicloudSlbClient.DescribeLoadBalancerTCPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
			}, runtime)
		if err != nil {
			return "", "", err
		}
		return tea.StringValue(resp.Body.AclStatus), tea.StringValue(resp.Body.AclId), nil
	case "udp":
		resp, err := r.client.DescribeLoadBalancerUDPListenerAttributeWithOptions(
			&alicloudSlbClient.DescribeLoadBalancerUDPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
			}, runtime)
		if err != nil {
			return "", "", err
		}
		return tea.StringValue(resp.Body.AclStatus), tea.StringValue(resp.Body.AclId), nil
	default:
		return "", "", fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

// readListenerAcl reads the ACL configuration from the listener attribute API.
// Returns (aclStatus, aclIds, error). Retries on transient errors.
func (r *slbListenerAclAttachmentResource) readListenerAcl(listenerId string) (string, []string, error) {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return "", nil, err
	}

	var aclStatus, aclIdStr string

	readFn := func() error {
		status, aclId, apiErr := r.describeListenerAcl(loadBalancerId, protocol, listenerPort)
		if apiErr != nil {
			if _t, ok := apiErr.(*tea.SDKError); ok && isAbleToRetry(*_t.Code) {
				return apiErr
			}
			return backoff.Permanent(apiErr)
		}
		aclStatus = status
		aclIdStr = aclId
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(readFn, reconnectBackoff)
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

	setAcl := func() error {
		apiErr := r.setListenerAclAttribute(
			loadBalancerId, protocol, listenerPort,
			aclStatus, tea.String(aclType), tea.String(aclIds),
		)
		if apiErr != nil {
			if isRetryableOrStatusError(apiErr) {
				return apiErr
			}
			return backoff.Permanent(apiErr)
		}
		return nil
	}

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(setAcl, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to set ACL on listener: %w", err)
	}

	return nil
}

func (r *slbListenerAclAttachmentResource) deleteAclConfig(listenerId string) error {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return err
	}

	// deleteAclConfig disables ACL on the listener by setting AclStatus="off".
	setAcl := func() error {
		apiErr := r.setListenerAclAttribute(
			loadBalancerId, protocol, listenerPort,
			"off", nil, nil,
		)
		// If failed to set status, it might due to listener is deleted or
		// listener is not ready to be perform any action
		if apiErr != nil {
			// Check is the listener being deleted
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

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(setAcl, reconnectBackoff)

	if err != nil {
		return fmt.Errorf("failed to disable ACL on listener: %w", err)
	}

	return nil
}

// setListenerAclAttribute calls the protocol-specific SetLoadBalancerListenerAttribute API.
// Pass nil for aclType/aclId to omit them from the request (used by delete).
func (r *slbListenerAclAttachmentResource) setListenerAclAttribute(loadBalancerId, protocol string, listenerPort int64, aclStatus string, aclType *string, aclId *string) error {
	runtime := &util.RuntimeOptions{}
	var apiErr error

	switch strings.ToLower(protocol) {
	case "http":
		_, apiErr = r.client.SetLoadBalancerHTTPListenerAttributeWithOptions(
			&alicloudSlbClient.SetLoadBalancerHTTPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String(aclStatus),
				AclType:        aclType,
				AclId:          aclId,
			}, runtime)
	case "https":
		_, apiErr = r.client.SetLoadBalancerHTTPSListenerAttributeWithOptions(
			&alicloudSlbClient.SetLoadBalancerHTTPSListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String(aclStatus),
				AclType:        aclType,
				AclId:          aclId,
			}, runtime)
	case "tcp":
		_, apiErr = r.client.SetLoadBalancerTCPListenerAttributeWithOptions(
			&alicloudSlbClient.SetLoadBalancerTCPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String(aclStatus),
				AclType:        aclType,
				AclId:          aclId,
			}, runtime)
	case "udp":
		_, apiErr = r.client.SetLoadBalancerUDPListenerAttributeWithOptions(
			&alicloudSlbClient.SetLoadBalancerUDPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String(aclStatus),
				AclType:        aclType,
				AclId:          aclId,
			}, runtime)
	default:
		return fmt.Errorf("unsupported protocol: %s, must be one of: http, https, tcp, udp", protocol)
	}

	return apiErr
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
	if sdkErr, ok := err.(*tea.SDKError); ok && sdkErr.Code != nil {
		code := strings.ToLower(*sdkErr.Code)
		if code == "operationfailed.listenerstatusnotsupport" {
			return true
		}
		return isAbleToRetry(*sdkErr.Code)
	}
	return false
}
