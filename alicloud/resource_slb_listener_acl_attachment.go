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

	var aclIdStrs []string
	diags := plan.AclIds.ElementsAs(ctx, &aclIdStrs, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	aclIds := strings.Join(aclIdStrs, ",")

	err := r.setListenerAclAttribute(plan.ListenerId.ValueString(), "on", tea.String("white"), tea.String(aclIds), false)
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

	var aclIdStrs []string
	diags := plan.AclIds.ElementsAs(ctx, &aclIdStrs, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	aclIds := strings.Join(aclIdStrs, ",")

	err := r.setListenerAclAttribute(plan.ListenerId.ValueString(), "on", tea.String("white"), tea.String(aclIds), false)
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

	err := r.setListenerAclAttribute(state.ListenerId.ValueString(), "off", nil, nil, true)
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

// readListenerAcl reads the ACL configuration using DescribeLoadBalancerListeners.
// Returns (aclStatus, aclIds, error). Retries on transient errors.
func (r *slbListenerAclAttachmentResource) readListenerAcl(listenerId string) (string, []string, error) {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return "", nil, err
	}

	var aclStatus, aclIdStr string

	readFn := func() error {
		resp, apiErr := r.client.DescribeLoadBalancerListenersWithOptions(
			&alicloudSlbClient.DescribeLoadBalancerListenersRequest{
				RegionId:         r.client.RegionId,
				LoadBalancerId:   []*string{tea.String(loadBalancerId)},
				ListenerProtocol: tea.String(protocol),
			}, &util.RuntimeOptions{})
		if apiErr != nil {
			if _t, ok := apiErr.(*tea.SDKError); ok && isAbleToRetry(*_t.Code) {
				return apiErr
			}
			return backoff.Permanent(apiErr)
		}
		if resp == nil || resp.Body == nil || resp.Body.Listeners == nil {
			return nil
		}
		for _, l := range resp.Body.Listeners {
			if tea.Int32Value(l.ListenerPort) == int32(listenerPort) {
				aclStatus = tea.StringValue(l.AclStatus)
				aclIdStr = tea.StringValue(l.AclId)
				break
			}
		}
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

// setListenerAclAttribute sets the ACL configuration on a listener via the protocol-specific API.
// Pass nil for aclType/aclId to omit them (used by delete). Retries on transient errors.
// If ignoreListenerGone is true, listener-not-found errors are silently swallowed (used by delete).
func (r *slbListenerAclAttachmentResource) setListenerAclAttribute(listenerId string, aclStatus string, aclType *string, aclId *string, ignoreListenerGone bool) error {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return err
	}

	runtime := &util.RuntimeOptions{}
	fn := func() error {
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
			return backoff.Permanent(fmt.Errorf("unsupported protocol: %s, must be one of: http, https, tcp, udp", protocol))
		}
		if apiErr != nil {
			if ignoreListenerGone {
				if sdkErr, ok := apiErr.(*tea.SDKError); ok && sdkErr.Code != nil {
					code := *sdkErr.Code
					if code == "InvalidListener" || code == "NoSuchListener" ||
						code == "InvalidLoadBalancerId.NotFound" || code == "ResourceNotFound" {
						return nil
					}
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
	if err = backoff.Retry(fn, bo); err != nil {
		if aclStatus == "off" {
			return fmt.Errorf("failed to disable ACL on listener: %w", err)
		}
		return fmt.Errorf("failed to set ACL on listener: %w", err)
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
	if sdkErr, ok := err.(*tea.SDKError); ok && sdkErr.Code != nil {
		code := strings.ToLower(*sdkErr.Code)
		if code == "operationfailed.listenerstatusnotsupport" {
			return true
		}
		return isAbleToRetry(*sdkErr.Code)
	}
	return false
}
