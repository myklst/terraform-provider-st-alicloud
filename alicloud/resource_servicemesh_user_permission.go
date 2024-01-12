package alicloud

import (
	"context"
	"reflect"

	// "strconv"
	"encoding/json"
	"time"

	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudServicemeshClient "github.com/alibabacloud-go/servicemesh-20200111/v4/client"
)

var (
	_ resource.Resource              = &servicemeshUserPermissionResource{}
	_ resource.ResourceWithConfigure = &servicemeshUserPermissionResource{}
)

func NewServicemeshUserPermissionResource() resource.Resource {
	return &servicemeshUserPermissionResource{}
}

type servicemeshUserPermissionResource struct {
	client *alicloudServicemeshClient.Client
}

type servicemeshUserPermissionModel struct {
	SubAccountUserId           types.String                  `tfsdk:"sub_account_user_id"`
	ServiceMeshUserPermissions []*serviceMeshUserPermissions `tfsdk:"permissions"`
}

type serviceMeshUserPermissions struct {
	ServiceMeshId types.String `tfsdk:"service_mesh_id"`
	IsCustom      types.Bool   `tfsdk:"is_custom"`
	RoleName      types.String `tfsdk:"role_name"`
	RoleType      types.String `tfsdk:"role_type"`
	IsRamRole     types.Bool   `tfsdk:"is_ram_role"`
}

type userPermissions struct {
	Cluster   string
	IsCustom  bool
	RoleName  string
	RoleType  string
	IsRamRole bool
}

// Metadata returns the Service Mesh User Permissions resource name.
func (r *servicemeshUserPermissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_mesh_user_permission"
}

// Schema defines the schema for the Service Mesh User Permissions resource.
func (r *servicemeshUserPermissionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Attach service mesh' role permissions (ASM) with a RAM user.",
		Attributes: map[string]schema.Attribute{
			"sub_account_user_id": schema.StringAttribute{
				Description: "The ID of the RAM user, and it can also be the id of the Ram Role. If you use Ram Role id, you need to set is_ram_role to true during authorization.",
				Required: true,
			},
		},
		Blocks: map[string]schema.Block{
			"permissions": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"service_mesh_id": schema.StringAttribute{
							Description: "The ID of the service mesh that you want to manage.",
							Optional: true,
						},
						"is_custom": schema.BoolAttribute{
							Description: "Specifies whether the grant object is a RAM role.",
							Optional: true,
						},
						"role_name": schema.StringAttribute{
							Description: "Specifies the predefined role that you want to assign. Valid values: [ istio-admin, istio-ops, istio-readonly ].",
							Optional: true,
							Validators: []validator.String{
								stringvalidator.OneOf("istio-admin", "istio-ops", "istio-readonly"),
							},
						},
						"role_type": schema.StringAttribute{
							Description: "The role type. Valid values: `custom`.",
							Optional: true,
							Validators: []validator.String{
								stringvalidator.OneOf("custom"),
							},
						},
						"is_ram_role": schema.BoolAttribute{
							Description: "Specifies whether the grant object is an entity.",
							Optional: true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *servicemeshUserPermissionResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).servicemeshClient
}

// Add Service Mesh user permissions with a RAM user.
func (r *servicemeshUserPermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan *servicemeshUserPermissionModel
	getStateDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Query the user's existing permissions
	existingPerms, err := r.describeUserPermissions(plan.SubAccountUserId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to query user's existing permission.",
			err.Error(),
		)
		return
	}

	// Append the existing permissions with the permission from plan result
	// Convert the permissions list to a Json String
	perms, err := json.Marshal(convertBaseTypeToPrimitiveDataType(append(existingPerms, plan.ServiceMeshUserPermissions...)))
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to convert the permissions list to a json string.",
			err.Error(),
		)
		return
	}

	// Grant permissions for user
	err = r.grantPermissions(plan.SubAccountUserId.ValueString(), string(perms))
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to grant permissions for user.",
			err.Error(),
		)
		return
	}

	// Set state items
	state := &servicemeshUserPermissionModel{
		SubAccountUserId: plan.SubAccountUserId,
		ServiceMeshUserPermissions: plan.ServiceMeshUserPermissions,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read function (Do nothing).
func (r *servicemeshUserPermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from state
	var state *servicemeshUserPermissionModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update the Service Mesh user permissions from a RAM user.
func (r *servicemeshUserPermissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan *servicemeshUserPermissionModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Retrieve values from state
	var state *servicemeshUserPermissionModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Query the user's existing permissions
	existingPerms, err := r.describeUserPermissions(plan.SubAccountUserId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to query user's existing permission.",
			err.Error(),
		)
		return
	}

	// Only remove the permissions from terraform state.
	var updatedPermission []*userPermissions
	for _, extPerm := range convertBaseTypeToPrimitiveDataType(existingPerms) {
		isExist := []bool{}
		for _, perm := range convertBaseTypeToPrimitiveDataType(state.ServiceMeshUserPermissions) {
			isExist = append(isExist, reflect.DeepEqual(extPerm, perm))
		}
		if isAllFalse(isExist) {
			updatedPermission = append(updatedPermission, extPerm)
		}
	}

	// Append the plan permissions with existing permissions
	// Convert the permissions list to a Json String
	perms, err := json.Marshal(append(updatedPermission, convertBaseTypeToPrimitiveDataType(plan.ServiceMeshUserPermissions)...))
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to convert the permissions list to a json string.",
			err.Error(),
		)
		return
	}

	// Grant permission for user
	err = r.grantPermissions(plan.SubAccountUserId.ValueString(), string(perms))
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to grant permissions for user.",
			err.Error(),
		)
		return
	}

	// Set state items
	state = &servicemeshUserPermissionModel{
		SubAccountUserId: plan.SubAccountUserId,
		ServiceMeshUserPermissions: plan.ServiceMeshUserPermissions,
	}

	// Set state to fully populated data
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Remove the CS kubernetes permissions from a RAM user.
func (r *servicemeshUserPermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state *servicemeshUserPermissionModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Query the user's existing permissions
	existingPerms, err := r.describeUserPermissions(state.SubAccountUserId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to query user's existing permission.",
			err.Error(),
		)
		return
	}

	// Only remove the permissions from terraform state.
	var preservedPerms []*userPermissions
	for _, extPerm := range convertBaseTypeToPrimitiveDataType(existingPerms) {
		isExist := []bool{}
		for _, perm := range convertBaseTypeToPrimitiveDataType(state.ServiceMeshUserPermissions) {
			isExist = append(isExist, reflect.DeepEqual(extPerm, perm))
		}
		if isAllFalse(isExist) {
			preservedPerms = append(preservedPerms, extPerm)
		}
	}

	// Convert the permissions list to a Json String
	perms, err := json.Marshal(preservedPerms)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to convert the permissions list to a json string.",
			err.Error(),
		)
		return
	}

	// Grant permission for user
	err = r.grantPermissions(state.SubAccountUserId.ValueString(), string(perms))
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to remove permissions for user.",
			err.Error(),
		)
		return
	}
}

func isAllFalse(list []bool) bool {
	for _, value := range list {
		if value == true {
			return false
		}
	}
	return true
}

// Convert basetype to primitive data type
func convertBaseTypeToPrimitiveDataType(baseTypeList []*serviceMeshUserPermissions) []*userPermissions {
	var primitiveDataTypeList []*userPermissions

	for _, value := range baseTypeList {
		primitiveDataTypeList = append(primitiveDataTypeList, &userPermissions{
			Cluster:   value.ServiceMeshId.ValueString(),
			IsCustom:  value.IsCustom.ValueBool(),
			RoleName:  value.RoleName.ValueString(),
			RoleType:  value.RoleType.ValueString(),
			IsRamRole: value.IsRamRole.ValueBool(),
		})
	}

	return primitiveDataTypeList
}

// Query user's existing permission
func (r *servicemeshUserPermissionResource) describeUserPermissions(uid string) ([]*serviceMeshUserPermissions, error) {
	var describeUserPermissionsResponse *alicloudServicemeshClient.DescribeUserPermissionsResponse
	var permissions []*serviceMeshUserPermissions
	var err error

	// Retry backoff function
	describeUserPermissions := func() error {
		runtime := &util.RuntimeOptions{}
		describeUserPermissionsRequest := &alicloudServicemeshClient.DescribeUserPermissionsRequest{
			SubAccountUserId: tea.String(uid),
		}
		describeUserPermissionsResponse, err = r.client.DescribeUserPermissionsWithOptions(describeUserPermissionsRequest, runtime)
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
	err = backoff.Retry(describeUserPermissions, reconnectBackoff)
	if err != nil {
		return permissions, err
	}

	for _, permission := range describeUserPermissionsResponse.Body.Permissions {
		perm := &serviceMeshUserPermissions{
			ServiceMeshId: types.StringValue(*permission.ResourceId),
			IsCustom:      types.BoolValue(true),
			RoleName:      types.StringValue(*permission.RoleName),
			RoleType:      types.StringValue(*permission.RoleType),
			IsRamRole:     types.BoolValue(false),
		}

		// check if the response returns the attribute IsRamRole
		// hasRamRole := reflect.ValueOf(permission).FieldByName("IsRamRole")
		// if hasRamRole.IsValid() {
		// 	isRamRole, err := strconv.ParseBool(*permission.IsRamRole)
		// 	if err != nil {
		// 		return permissions, err
		// 	}
		// 	perm.IsRamRole = types.BoolValue(isRamRole)
		// }

		permissions = append(permissions, perm)
	}

	return permissions, nil
}

// Grant Service Mesh permissions for user
func (r *servicemeshUserPermissionResource) grantPermissions(uid string, permString string) error {
	var err error

	// Retry backoff function
	grantPermissions := func() error {
		runtime := &util.RuntimeOptions{}

		grantUserPermissionsRequest := &alicloudServicemeshClient.GrantUserPermissionsRequest{
			SubAccountUserId: tea.String(uid),
			Permissions: tea.String(permString),
		}

		_, err = r.client.GrantUserPermissionsWithOptions(grantUserPermissionsRequest, runtime)
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
