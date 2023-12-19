package alicloud

import (
	"context"
	"reflect"
	"strings"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudCsClient "github.com/alibabacloud-go/cs-20151215/v4/client"
)

var (
	_ resource.Resource              = &csKubernetesPermissionsResource{}
	_ resource.ResourceWithConfigure = &csKubernetesPermissionsResource{}
)

func NewCsKubernetesPermissionsResource() resource.Resource {
	return &csKubernetesPermissionsResource{}
}

type csKubernetesPermissionsResource struct {
	client *alicloudCsClient.Client
}

type csKubernetesPermissionsModel struct {
	Uid         types.String   `tfsdk:"uid"`
	Permissions []*permissions `tfsdk:"permissions"`
}

type permissions struct {
	Cluster   types.String `tfsdk:"cluster"`
	IsCustom  types.Bool   `tfsdk:"is_custom"`
	RoleName  types.String `tfsdk:"role_name"`
	RoleType  types.String `tfsdk:"role_type"`
	Namespace types.String `tfsdk:"namespace"`
	IsRamRole types.Bool   `tfsdk:"is_ram_role"`
}

// Metadata returns the CS Kubernetes Permissions resource name.
func (r *csKubernetesPermissionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cs_kubernetes_permissions"
}

// Schema defines the schema for the CS Kubernetes Permissions resource.
func (r *csKubernetesPermissionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attach clusters' kubernetes role permissions (CS) with a RAM user.",
		Attributes: map[string]schema.Attribute{
			"uid": schema.StringAttribute{
				Description: "The ID of the Ram user, and it can also be the id of the Ram Role. If you use Ram Role id, you need to set is_ram_role to true during authorization.",
				Required: true,
			},
		},
		Blocks: map[string]schema.Block{
			"permissions": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"cluster": schema.StringAttribute{
							Description: "The ID of the cluster that you want to manage.",
							Required: true,
						},
						"is_custom": schema.BoolAttribute{
							Description: "Specifies whether to perform a custom authorization. To perform a custom authorization, set role_name to a custom cluster role.",
							Optional: true,
						},
						"role_name": schema.StringAttribute{
							Description: "Specifies the predefined role that you want to assign. Valid values: [ admin, ops, dev, restricted and the custom cluster roles ].",
							Required: true,
						},
						"role_type": schema.StringAttribute{
							Description: "The authorization type. Valid values: [ cluster, namespace, all-clusters ].",
							Required:     true,
							Validators: []validator.String{
								stringvalidator.OneOf("cluster", "namespace", "all-clusters"),
							},
						},
						"namespace": schema.StringAttribute{
							Description: "The namespace to which the permissions are scoped. This parameter is required only if you set role_type to namespace.",
							Optional: true,
						},

						"is_ram_role": schema.BoolAttribute{
							Description: "Specifies whether the permissions are granted to a RAM role. When uid is ram role id, the value of is_ram_role must be true.",
							Optional: true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *csKubernetesPermissionsResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).csClient
}

// Add CS kubernetes permissions with a RAM user.
func (r *csKubernetesPermissionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *csKubernetesPermissionsModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Query the user's existing permissions
	perms, err := r.describeUserPermission(plan.Uid.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to query user's existing permission.",
			err.Error(),
		)
		return
	}

	// Append the existing permissions with the permission from plan result
	perms = append(perms, convertPermissionsValueToGrantPermissionsRequestBody(plan.Permissions)...)

	// Grant permissions for user
	err = r.grantPermissions(plan.Uid.ValueString(), perms)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to grant permissions for user.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &csKubernetesPermissionsModel{
		Uid: plan.Uid,
		Permissions: plan.Permissions,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read function (Do nothing).
func (r *csKubernetesPermissionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from state
	var state *csKubernetesPermissionsModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update the CS kubernetes permissions from a RAM user.
func (r *csKubernetesPermissionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan *csKubernetesPermissionsModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Retrieve values from state
	var state *csKubernetesPermissionsModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Query the user's existing permissions
	existing_perms, err := r.describeUserPermission(plan.Uid.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to query user's existing permission.",
			err.Error(),
		)
		return
	}

	// Only remove the permissions from terraform state.
	var updatedPermission []*alicloudCsClient.GrantPermissionsRequestBody
	var isExist []bool
	for _, extPerm := range existing_perms {
		for _, perm := range convertPermissionsValueToGrantPermissionsRequestBody(state.Permissions) {
			isExist = append(isExist, reflect.DeepEqual(extPerm, perm))
		}
		if allFalse(isExist) {
			updatedPermission = append(updatedPermission, extPerm)
		}
	}

	// Then append the plan permissions with existing permissions
	updatedPermission = append(updatedPermission, convertPermissionsValueToGrantPermissionsRequestBody(plan.Permissions)...)

	// Grant permission for user
	err = r.grantPermissions(plan.Uid.ValueString(), updatedPermission)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to grant permissions for user.",
			err.Error(),
		)
		return
	}

	// Set state items
	state = &csKubernetesPermissionsModel{
		Uid: plan.Uid,
		Permissions: plan.Permissions,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Remove the CS kubernetes permissions from a RAM user.
func (r *csKubernetesPermissionsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *csKubernetesPermissionsModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Query the user's existing permissions
	existing_perms, err := r.describeUserPermission(state.Uid.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to query user's existing permission.",
			err.Error(),
		)
		return
	}

	// Only remove the permissions from terraform state.
	var preserved_perms []*alicloudCsClient.GrantPermissionsRequestBody
	var isExist []bool
	for _, extPerm := range existing_perms {
		for _, perm := range convertPermissionsValueToGrantPermissionsRequestBody(state.Permissions) {
			isExist = append(isExist, reflect.DeepEqual(extPerm, perm))
		}
		if allFalse(isExist) {
			preserved_perms = append(preserved_perms, extPerm)
		}
	}

	// Grant permission for user
	err = r.grantPermissions(state.Uid.ValueString(), preserved_perms)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to remove permissions for user.",
			err.Error(),
		)
		return
	}
}

func allFalse(list []bool) bool {
	for _, value := range list {
		if value == true {
			return false
		}
	}
	return true
}

func convertPermissionsValueToGrantPermissionsRequestBody(perms []*permissions) []*alicloudCsClient.GrantPermissionsRequestBody {
	var request []*alicloudCsClient.GrantPermissionsRequestBody

	for _, perm := range perms {
		request = append(request, &alicloudCsClient.GrantPermissionsRequestBody{
			Cluster:   tea.String(perm.Cluster.ValueString()),
			IsCustom:  tea.Bool(perm.IsCustom.ValueBool()),
			RoleName:  tea.String(perm.RoleName.ValueString()),
			RoleType:  tea.String(perm.RoleType.ValueString()),
			Namespace: tea.String(perm.Namespace.ValueString()),
			IsRamRole: tea.Bool(perm.IsRamRole.ValueBool()),
		})
	}

	return request
}

// Query user's existing permission
func (r *csKubernetesPermissionsResource) describeUserPermission(uid string) ([]*alicloudCsClient.GrantPermissionsRequestBody, error) {
	var describeUserPermissionResponse *alicloudCsClient.DescribeUserPermissionResponse
	var permissions []*alicloudCsClient.GrantPermissionsRequestBody
	var err error

	// Retry backoff function
	describeUserPermission := func() error {
		runtime := &util.RuntimeOptions{}
		headers := make(map[string]*string)
		describeUserPermissionResponse, err = r.client.DescribeUserPermissionWithOptions(tea.String(uid), headers, runtime)
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

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(describeUserPermission, reconnectBackoff)
	if err != nil {
		return permissions, err
	}

	for _, permission := range describeUserPermissionResponse.Body {
		perm := &alicloudCsClient.GrantPermissionsRequestBody{
			Cluster:   nil,
			IsCustom:  nil,
			RoleName:  nil,
			RoleType:  tea.String("cluster"),
			Namespace: tea.String(""),
			IsRamRole: nil,
		}
		resourceId := tea.StringValue(permission.ResourceId)
		resourceType := tea.StringValue(permission.ResourceType)
		perm.IsRamRole = tea.Bool(tea.Int64Value(permission.IsRamRole) == 1)

		if tea.StringValue(permission.RoleType) == "custom" {
			perm.IsCustom = tea.Bool(true)
			perm.RoleName = tea.String(tea.StringValue(permission.RoleName))
		} else {
			perm.RoleName = tea.String(tea.StringValue(permission.RoleType))
		}

		if strings.Contains(resourceId, "/") {
			parts := strings.Split(resourceId, "/")
			cluster := parts[0]
			namespace := parts[1]
			perm.Cluster = tea.String(cluster)
			perm.Namespace = tea.String(namespace)
			perm.RoleType = tea.String("namespace")
		} else if resourceType == "cluster" {
			cluster := resourceId
			perm.Cluster = tea.String(cluster)
			perm.RoleType = tea.String("cluster")
		}

		if resourceType == "console" && resourceId == "all-clusters" {
			perm.RoleType = tea.String("all-clusters")
		}

		permissions = append(permissions, perm)
	}

	return permissions, nil
}

// Grant kubernetes permission for user
func (r *csKubernetesPermissionsResource) grantPermissions(uid string, request []*alicloudCsClient.GrantPermissionsRequestBody) error {
	var err error

	// Retry backoff function
	grantPermissions := func() error {
		runtime := &util.RuntimeOptions{}
		headers := make(map[string]*string)

		grantPermissionsRequest := &alicloudCsClient.GrantPermissionsRequest{
			Body: request,
		}

		_, err = r.client.GrantPermissionsWithOptions(tea.String(uid), grantPermissionsRequest, headers, runtime)
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

	// Retry backoff
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(grantPermissions, reconnectBackoff)
	if err != nil {
		return err
	}

	return nil
}
