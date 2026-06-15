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

	err := r.setAclConfig(plan.ListenerId.ValueString(), plan.AclIds)
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

	var aclStatus string
	var sourceItems string

	readAcl := func() error {
		runtime := &util.RuntimeOptions{}

		request := &alicloudSlbClient.DescribeListenerAccessControlAttributeRequest{
			LoadBalancerId:   tea.String(loadBalancerId),
			ListenerPort:     tea.Int32(int32(listenerPort)),
			ListenerProtocol: tea.String(protocol),
		}

		response, err := r.client.DescribeListenerAccessControlAttributeWithOptions(request, runtime)
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

		aclStatus = tea.ToString(response.Body.AccessControlStatus)
		sourceItems = tea.ToString(response.Body.SourceItems)

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(readAcl, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to read SLB listener ACL attribute.",
			err.Error(),
		)
		return
	}

	// If ACL is off, the attachment is effectively gone
	if aclStatus == "off" || aclStatus == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	// Parse acl_ids from SourceItems (comma-separated string)
	var aclIds []string
	if sourceItems != "" {
		aclIds = strings.Split(sourceItems, ",")
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

	err := r.setAclConfig(plan.ListenerId.ValueString(), plan.AclIds)
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

// Delete removes the ACL attachment by turning off access control.
func (r *slbListenerAclAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *slbListenerAclAttachmentModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	listenerId := state.ListenerId.ValueString()
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		resp.Diagnostics.AddError("Invalid listener_id", err.Error())
		return
	}

	disableAcl := func() error {
		runtime := &util.RuntimeOptions{}

		request := &alicloudSlbClient.SetListenerAccessControlStatusRequest{
			LoadBalancerId:      tea.String(loadBalancerId),
			ListenerPort:        tea.Int32(int32(listenerPort)),
			ListenerProtocol:    tea.String(protocol),
			AccessControlStatus: tea.String("off"),
		}

		_, err := r.client.SetListenerAccessControlStatusWithOptions(request, runtime)
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
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(disableAcl, reconnectBackoff)
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

// setAclConfig enables access control and sets ACL type + IDs on the listener.
func (r *slbListenerAclAttachmentResource) setAclConfig(listenerId string, aclIdsList types.List) error {
	loadBalancerId, protocol, listenerPort, err := parseListenerId(listenerId)
	if err != nil {
		return err
	}

	// Convert acl_ids list to comma-separated string
	var aclIdStrs []string
	for _, id := range aclIdsList.Elements() {
		aclIdStrs = append(aclIdStrs, trimStringQuotes(id.String()))
	}
	aclIds := strings.Join(aclIdStrs, ",")

	// Step 1: Enable access control on the listener
	enableAcl := func() error {
		runtime := &util.RuntimeOptions{}

		request := &alicloudSlbClient.SetListenerAccessControlStatusRequest{
			LoadBalancerId:      tea.String(loadBalancerId),
			ListenerPort:        tea.Int32(int32(listenerPort)),
			ListenerProtocol:    tea.String(protocol),
			AccessControlStatus: tea.String("on"),
		}

		_, err := r.client.SetListenerAccessControlStatusWithOptions(request, runtime)
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
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(enableAcl, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to enable access control: %w", err)
	}

	// Step 2: Set ACL type and ACL IDs using the protocol-specific SetListenerAttribute API
	setAcl := func() error {
		runtime := &util.RuntimeOptions{}

		switch strings.ToLower(protocol) {
		case "http":
			request := &alicloudSlbClient.SetLoadBalancerHTTPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String("on"),
				AclType:        tea.String("white"),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerHTTPListenerAttributeWithOptions(request, runtime)
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
		case "https":
			request := &alicloudSlbClient.SetLoadBalancerHTTPSListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String("on"),
				AclType:        tea.String("white"),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerHTTPSListenerAttributeWithOptions(request, runtime)
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
		case "tcp":
			request := &alicloudSlbClient.SetLoadBalancerTCPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String("on"),
				AclType:        tea.String("white"),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerTCPListenerAttributeWithOptions(request, runtime)
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
		case "udp":
			request := &alicloudSlbClient.SetLoadBalancerUDPListenerAttributeRequest{
				LoadBalancerId: tea.String(loadBalancerId),
				ListenerPort:   tea.Int32(int32(listenerPort)),
				AclStatus:      tea.String("on"),
				AclType:        tea.String("white"),
				AclId:          tea.String(aclIds),
			}
			_, err := r.client.SetLoadBalancerUDPListenerAttributeWithOptions(request, runtime)
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
		default:
			return backoff.Permanent(fmt.Errorf("unsupported protocol: %s, must be one of: http, https, tcp, udp", protocol))
		}

		return nil
	}

	reconnectBackoff = backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(setAcl, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to set ACL on listener: %w", err)
	}

	return nil
}
