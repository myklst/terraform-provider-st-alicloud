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

// isListenerStatusError returns true if the error is a listener-not-ready error.
func isListenerStatusError(err error) bool {
	if sdkErr, ok := err.(*tea.SDKError); ok && sdkErr.Code != nil {
		return strings.ToLower(*sdkErr.Code) == "operationfailed.listenerstatusnotsupport"
	}
	return false
}

// isRetryableOrStatusError returns true if the error is retryable (from the global list)
// or is a listener status error (which is transient and should be retried).
func isRetryableOrStatusError(err error) bool {
	if isListenerStatusError(err) {
		return true
	}
	if sdkErr, ok := err.(*tea.SDKError); ok && sdkErr.Code != nil {
		return isAbleToRetry(*sdkErr.Code)
	}
	return false
}

// stringVal safely dereferences a *string pointer, returning "" for nil.
func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// aclIdsFromList converts a types.List of strings to a []string using ElementsAs.
func aclIdsFromList(ctx context.Context, aclIdsList types.List) ([]string, error) {
	var result []string
	diags := aclIdsList.ElementsAs(ctx, &result, false)
	if diags.HasError() {
		return nil, fmt.Errorf("failed to convert acl_ids list: %v", diags)
	}
	return result, nil
}

// readListenerAcl reads the ACL configuration from the listener attribute API.
// Returns (aclStatus, aclType, aclIds, error).
func (r *slbListenerAclAttachmentResource) readListenerAcl(loadBalancerId, protocol string, listenerPort int64) (string, string, []string, error) {
	var aclStatus, aclType, aclIdStr string

	readFn := func() error {
		runtime := &util.RuntimeOptions{}

		switch strings.ToLower(protocol) {
		case "http":
			req := &alicloudSlbClient.DescribeLoadBalancerHTTPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
			}
			resp, err := r.client.DescribeLoadBalancerHTTPListenerAttributeWithOptions(req, runtime)
			if err != nil {
				if _t, ok := err.(*tea.SDKError); ok && isAbleToRetry(*_t.Code) {
					return err
				}
				return backoff.Permanent(err)
			}
			aclStatus = stringVal(resp.Body.AclStatus)
			aclType = stringVal(resp.Body.AclType)
			aclIdStr = stringVal(resp.Body.AclId)
		case "https":
			req := &alicloudSlbClient.DescribeLoadBalancerHTTPSListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
			}
			resp, err := r.client.DescribeLoadBalancerHTTPSListenerAttributeWithOptions(req, runtime)
			if err != nil {
				if _t, ok := err.(*tea.SDKError); ok && isAbleToRetry(*_t.Code) {
					return err
				}
				return backoff.Permanent(err)
			}
			aclStatus = stringVal(resp.Body.AclStatus)
			aclType = stringVal(resp.Body.AclType)
			aclIdStr = stringVal(resp.Body.AclId)
		case "tcp":
			req := &alicloudSlbClient.DescribeLoadBalancerTCPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
			}
			resp, err := r.client.DescribeLoadBalancerTCPListenerAttributeWithOptions(req, runtime)
			if err != nil {
				if _t, ok := err.(*tea.SDKError); ok && isAbleToRetry(*_t.Code) {
					return err
				}
				return backoff.Permanent(err)
			}
			aclStatus = stringVal(resp.Body.AclStatus)
			aclType = stringVal(resp.Body.AclType)
			aclIdStr = stringVal(resp.Body.AclId)
		case "udp":
			req := &alicloudSlbClient.DescribeLoadBalancerUDPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
			}
			resp, err := r.client.DescribeLoadBalancerUDPListenerAttributeWithOptions(req, runtime)
			if err != nil {
				if _t, ok := err.(*tea.SDKError); ok && isAbleToRetry(*_t.Code) {
					return err
				}
				return backoff.Permanent(err)
			}
			aclStatus = stringVal(resp.Body.AclStatus)
			aclType = stringVal(resp.Body.AclType)
			aclIdStr = stringVal(resp.Body.AclId)
		default:
			return backoff.Permanent(fmt.Errorf("unsupported protocol: %s", protocol))
		}
		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(readFn, bo)
	if err != nil {
		return "", "", nil, err
	}

	// AclId is comma-separated string, split into list
	var aclIds []string
	if aclIdStr != "" {
		aclIds = strings.Split(aclIdStr, ",")
	}
	return aclStatus, aclType, aclIds, nil
}

// Metadata returns the SLB Listener ACL Attachment resource name.
func (r *slbListenerAclAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_slb_listener_acl_attachment"
}

// Schema defines the schema for the SLB Listener ACL Attachment resource.
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

// Configure adds the provider configured client to the resource.
func (r *slbListenerAclAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).slbClient
}

// Create attaches ACLs to the SLB listener.
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
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read reads the SLB listener ACL attachment state.
func (r *slbListenerAclAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *slbListenerAclAttachmentModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	listenerId := state.ListenerId.ValueString()
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		resp.Diagnostics.AddError("Invalid listener_id", err.Error())
		return
	}

	_, _, aclIds, err := r.readListenerAcl(loadBalancerId, protocol, listenerPort)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read SLB listener ACL attribute.",
			err.Error(),
		)
		return
	}

	// If no ACL IDs attached, the attachment is gone
	if len(aclIds) == 0 {
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
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the SLB listener ACL attachment.
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
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete removes the ACL attachment by turning off access control via SetListenerAttribute.
func (r *slbListenerAclAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *slbListenerAclAttachmentModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build an empty acl_ids list for delete
	emptyAclIds, diags := types.ListValueFrom(ctx, types.StringType, []string{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.deleteAclConfig(ctx, state.ListenerId.ValueString(), emptyAclIds)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to disable SLB listener access control.",
			err.Error(),
		)
		return
	}
}

// ImportState imports an existing SLB listener ACL attachment.
func (r *slbListenerAclAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("listener_id"), req, resp)
}

// setAclConfig sets the ACL configuration on the SLB listener using SetListenerAttribute.
// First waits for the listener to be in "running" status, then applies the ACL config.
// Retries on OperationFailed.ListenerStatusNotSupport since that's a transient error.
func (r *slbListenerAclAttachmentResource) setAclConfig(ctx context.Context, listenerId string, aclIdsList types.List, aclStatus string, aclType string) error {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return err
	}

	// Convert acl_ids list to comma-separated string using ElementsAs
	aclIdStrs, err := aclIdsFromList(ctx, aclIdsList)
	if err != nil {
		return err
	}
	aclIds := strings.Join(aclIdStrs, ",")

	setAcl := func() error {
		runtime := &util.RuntimeOptions{}

		switch strings.ToLower(protocol) {
		case "http":
			request := &alicloudSlbClient.SetLoadBalancerHTTPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String(aclStatus),
				AclType:        tea.String(aclType),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerHTTPListenerAttributeWithOptions(request, runtime)
			if err != nil {
				if isRetryableOrStatusError(err) {
					return err
				}
				return backoff.Permanent(err)
			}
		case "https":
			request := &alicloudSlbClient.SetLoadBalancerHTTPSListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String(aclStatus),
				AclType:        tea.String(aclType),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerHTTPSListenerAttributeWithOptions(request, runtime)
			if err != nil {
				if isRetryableOrStatusError(err) {
					return err
				}
				return backoff.Permanent(err)
			}
		case "tcp":
			request := &alicloudSlbClient.SetLoadBalancerTCPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String(aclStatus),
				AclType:        tea.String(aclType),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerTCPListenerAttributeWithOptions(request, runtime)
			if err != nil {
				if isRetryableOrStatusError(err) {
					return err
				}
				return backoff.Permanent(err)
			}
		case "udp":
			request := &alicloudSlbClient.SetLoadBalancerUDPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String(aclStatus),
				AclType:        tea.String(aclType),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerUDPListenerAttributeWithOptions(request, runtime)
			if err != nil {
				if isRetryableOrStatusError(err) {
					return err
				}
				return backoff.Permanent(err)
			}
		default:
			return backoff.Permanent(fmt.Errorf("unsupported protocol: %s, must be one of: http, https, tcp, udp", protocol))
		}

		return nil
	}

	// Retry with longer initial interval — the listener may still be
	// transitioning after creation, causing ListenerStatusNotSupport.
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 5 * time.Minute
	bo.InitialInterval = 15 * time.Second
	err = backoff.Retry(setAcl, bo)
	if err != nil {
		return fmt.Errorf("failed to set ACL on listener: %w", err)
	}

	return nil
}

// deleteAclConfig disables ACL on the listener. Unlike setAclConfig, it does NOT
// wait for listener to be running — during destroy the listener may be shutting down.
// If the listener is already gone, treat as success.
func (r *slbListenerAclAttachmentResource) deleteAclConfig(ctx context.Context, listenerId string, aclIdsList types.List) error {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return err
	}

	// Convert acl_ids list to comma-separated string using ElementsAs
	aclIdStrs, err := aclIdsFromList(ctx, aclIdsList)
	if err != nil {
		return err
	}
	aclIds := strings.Join(aclIdStrs, ",")

	// No waitForListenerReady on delete — listener may be in transitional/shutting-down state
	setAcl := func() error {
		runtime := &util.RuntimeOptions{}

		switch strings.ToLower(protocol) {
		case "http":
			request := &alicloudSlbClient.SetLoadBalancerHTTPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String("off"),
				AclType:        tea.String(""),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerHTTPListenerAttributeWithOptions(request, runtime)
			if err != nil {
				if isListenerGoneError(err) {
					return nil // listener already gone, delete succeeded
				}
				if isRetryableOrStatusError(err) {
					return err
				}
				return backoff.Permanent(err)
			}
		case "https":
			request := &alicloudSlbClient.SetLoadBalancerHTTPSListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String("off"),
				AclType:        tea.String(""),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerHTTPSListenerAttributeWithOptions(request, runtime)
			if err != nil {
				if isListenerGoneError(err) {
					return nil
				}
				if isRetryableOrStatusError(err) {
					return err
				}
				return backoff.Permanent(err)
			}
		case "tcp":
			request := &alicloudSlbClient.SetLoadBalancerTCPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String("off"),
				AclType:        tea.String(""),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerTCPListenerAttributeWithOptions(request, runtime)
			if err != nil {
				if isListenerGoneError(err) {
					return nil
				}
				if isRetryableOrStatusError(err) {
					return err
				}
				return backoff.Permanent(err)
			}
		case "udp":
			request := &alicloudSlbClient.SetLoadBalancerUDPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String("off"),
				AclType:        tea.String(""),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerUDPListenerAttributeWithOptions(request, runtime)
			if err != nil {
				if isListenerGoneError(err) {
					return nil
				}
				if isRetryableOrStatusError(err) {
					return err
				}
				return backoff.Permanent(err)
			}
		default:
			return backoff.Permanent(fmt.Errorf("unsupported protocol: %s, must be one of: http, https, tcp, udp", protocol))
		}

		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 2 * time.Minute
	bo.InitialInterval = 3 * time.Second
	err = backoff.Retry(setAcl, bo)
	if err != nil {
		return fmt.Errorf("failed to disable ACL on listener: %w", err)
	}

	return nil
}

// isListenerGoneError returns true if the error indicates the listener no longer exists.
func isListenerGoneError(err error) bool {
	if sdkErr, ok := err.(*tea.SDKError); ok && sdkErr.Code != nil {
		code := *sdkErr.Code
		// Listener not found or SLB not found
		return code == "InvalidListener" || code == "NoSuchListener" ||
			code == "InvalidLoadBalancerId.NotFound" || code == "ResourceNotFound"
	}
	return false
}
