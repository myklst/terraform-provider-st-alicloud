package alicloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudRamClient "github.com/alibabacloud-go/ram-20150501/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

const maxLength = 6144

var (
	_ resource.Resource                = &ramPolicyResource{}
	_ resource.ResourceWithConfigure   = &ramPolicyResource{}
	_ resource.ResourceWithImportState = &ramPolicyResource{}
)

func NewRamPolicyResource() resource.Resource {
	return &ramPolicyResource{}
}

type ramPolicyResource struct {
	client *alicloudRamClient.Client
}

type ramPolicyResourceModel struct {
	AttachedPolicies types.List   `tfsdk:"attached_policies"`
	Policies         types.List   `tfsdk:"policies"`
	UserName         types.String `tfsdk:"user_name"`
}

type policyDetail struct {
	PolicyName     types.String `tfsdk:"policy_name"`
	PolicyDocument types.String `tfsdk:"policy_document"`
}

func (r *ramPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ram_policy"
}

func (r *ramPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a RAM Policy resource that manages policy content exceeding character limits by splitting it into smaller segments. These segments are combined to form a complete policy attached to the user.",
		Attributes: map[string]schema.Attribute{
			"attached_policies": schema.ListAttribute{
				Description: "The RAM policies to attach to the user.",
				Required:    true,
				ElementType: types.StringType,
			},
			"policies": schema.ListNestedAttribute{
				Description: "A list of policies.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"policy_name": schema.StringAttribute{
							Description: "The policy name.",
							Computed:    true,
						},
						"policy_document": schema.StringAttribute{
							Description: "The policy document of the RAM policy.",
							Computed:    true,
						},
					},
				},
			},
			"user_name": schema.StringAttribute{
				Description: "The name of the RAM user that attached to the policy.",
				Required:    true,
			},
		},
	}
}

func (r *ramPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(alicloudClients).ramClient
}

func (r *ramPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *ramPolicyResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.createPolicy(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create the Policy.",
			err.Error(),
		)
		return
	}

	state := &ramPolicyResourceModel{}
	state.AttachedPolicies = plan.AttachedPolicies
	state.Policies = types.ListValueMust(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"policy_name":     types.StringType,
				"policy_document": types.StringType,
			},
		},
		policy,
	)
	state.UserName = plan.UserName

	if err := r.attachPolicyToUser(state); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Attach Policy to User.",
			err.Error(),
		)
		return
	}

	readPolicyDiags := r.readPolicy(state)
	resp.Diagnostics.Append(readPolicyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ramPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *ramPolicyResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readPolicyDiags := r.readPolicy(state)
	resp.Diagnostics.Append(readPolicyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	listPoliciesForUser := func() error {
		runtime := &util.RuntimeOptions{}

		listPoliciesForUserRequest := &alicloudRamClient.ListPoliciesForUserRequest{
			UserName: tea.String(state.UserName.ValueString()),
		}

		_, err := r.client.ListPoliciesForUserWithOptions(listPoliciesForUserRequest, runtime)
		if err != nil {
			handleAPIError(err)
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(listPoliciesForUser, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Read Users for Group",
			err.Error(),
		)
		return
	}

	// This state will be using to compare with the current state.
	var oriState *ramPolicyResourceModel
	getOriStateDiags := req.State.Get(ctx, &oriState)
	resp.Diagnostics.Append(getOriStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(state.Policies.Elements()) != len(oriState.Policies.Elements()) {
		resp.Diagnostics.AddWarning("Combined policies not found.", "The combined policies attached to the user may be deleted due to human mistake or API error.")
		state.AttachedPolicies = types.ListNull(types.StringType)
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ramPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state *ramPolicyResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	removePolicyDiags := r.removePolicy(state)
	resp.Diagnostics.Append(removePolicyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.createPolicy(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Update the Policy.",
			err.Error(),
		)
		return
	}

	state.AttachedPolicies = plan.AttachedPolicies
	state.Policies = types.ListValueMust(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"policy_name":     types.StringType,
				"policy_document": types.StringType,
			},
		},
		policy,
	)
	state.UserName = plan.UserName

	if err := r.attachPolicyToUser(state); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Attach Policy to User.",
			err.Error(),
		)
		return
	}

	readPolicyDiags := r.readPolicy(state)
	resp.Diagnostics.Append(readPolicyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ramPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *ramPolicyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	removePolicyDiags := r.removePolicy(state)
	resp.Diagnostics.Append(removePolicyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ramPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	policyDetailsState := []*policyDetail{}
	getPolicyResponse := &alicloudRamClient.GetPolicyResponse{}
	policyNames := strings.Split(req.ID, ",")
	var username string

	var err error
	getPolicy := func() error {
		runtime := &util.RuntimeOptions{}

		for _, policyName := range policyNames {
			policyName = strings.ReplaceAll(policyName, " ", "")

			// Retrieves the policy document for the policy
			getPolicyRequest := &alicloudRamClient.GetPolicyRequest{
				PolicyName: tea.String(policyName),
				PolicyType: tea.String("Custom"),
			}

			getPolicyResponse, err = r.client.GetPolicyWithOptions(getPolicyRequest, runtime)
			if err != nil {
				handleAPIError(err)
			}

			// Retrieves the name of the user attached to the policy.
			listEntitiesForPolicy := &alicloudRamClient.ListEntitiesForPolicyRequest{
				PolicyName: tea.String(policyName),
				PolicyType: tea.String("Custom"),
			}

			getPolicyEntities, err := r.client.ListEntitiesForPolicyWithOptions(listEntitiesForPolicy, runtime)
			if err != nil {
				handleAPIError(err)
			}

			if getPolicyResponse.Body.Policy != nil {
				policyDetail := policyDetail{
					PolicyName:     types.StringValue(*getPolicyResponse.Body.Policy.PolicyName),
					PolicyDocument: types.StringValue(*getPolicyResponse.Body.DefaultPolicyVersion.PolicyDocument),
				}
				policyDetailsState = append(policyDetailsState, &policyDetail)
			}

			if getPolicyEntities.Body.Users != nil {
				for _, user := range getPolicyEntities.Body.Users.User {
					username = *user.UserName
				}
			}
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(getPolicy, reconnectBackoff)
	if err != nil {
		return
	}

	var policyList []policyDetail
	for _, policy := range policyDetailsState {
		policies := policyDetail{
			PolicyName:     types.StringValue(policy.PolicyName.ValueString()),
			PolicyDocument: types.StringValue(policy.PolicyDocument.ValueString()),
		}

		policyList = append(policyList, policies)
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_name"), username)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("policies"), policyList)...)

	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning(
			"Unable to Set the attached_policies Attribute",
			"After running terraform import, Terraform will not automatically set the attached_policies attributes."+
				"To ensure that all attributes defined in the Terraform configuration are set, you need to run terraform apply."+
				"This command will apply the changes and set the desired attributes according to your configuration.",
		)
	}
}

func (r *ramPolicyResource) createPolicy(plan *ramPolicyResourceModel) (policiesList []attr.Value, err error) {
	formattedPolicy, err := r.getPolicyDocument(plan)
	if err != nil {
		return nil, err
	}

	createPolicy := func() error {
		runtime := &util.RuntimeOptions{}

		for i, policy := range formattedPolicy {
			policyName := plan.UserName.ValueString() + "-" + strconv.Itoa(i+1)

			createPolicyRequest := &alicloudRamClient.CreatePolicyRequest{
				PolicyName:     tea.String(policyName),
				PolicyDocument: tea.String(policy),
			}

			if _, err := r.client.CreatePolicyWithOptions(createPolicyRequest, runtime); err != nil {
				handleAPIError(err)
			}
		}

		return nil
	}

	for i, policies := range formattedPolicy {
		policyName := plan.UserName.ValueString() + "-" + strconv.Itoa(i+1)

		policyObj := types.ObjectValueMust(
			map[string]attr.Type{
				"policy_name":     types.StringType,
				"policy_document": types.StringType,
			},
			map[string]attr.Value{
				"policy_name":     types.StringValue(policyName),
				"policy_document": types.StringValue(policies),
			},
		)
		policiesList = append(policiesList, policyObj)
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	return policiesList, backoff.Retry(createPolicy, reconnectBackoff)
}

func (r *ramPolicyResource) readPolicy(state *ramPolicyResourceModel) diag.Diagnostics {
	policyDetailsState := []*policyDetail{}
	getPolicyResponse := &alicloudRamClient.GetPolicyResponse{}

	var err error
	getPolicy := func() error {
		runtime := &util.RuntimeOptions{}

		data := make(map[string]string)

		for _, policies := range state.Policies.Elements() {
			json.Unmarshal([]byte(policies.String()), &data)

			getPolicyRequest := &alicloudRamClient.GetPolicyRequest{
				PolicyName: tea.String(data["policy_name"]),
				PolicyType: tea.String("Custom"),
			}

			getPolicyResponse, err = r.client.GetPolicyWithOptions(getPolicyRequest, runtime)
			if err != nil {
				handleAPIError(err)
			}

			// Sometimes combined policies may be removed accidentally by human mistake or API error.
			if getPolicyResponse.Body != nil && getPolicyResponse.Body.Policy != nil {
				if getPolicyResponse.Body.Policy.PolicyName != nil && getPolicyResponse.Body.DefaultPolicyVersion.PolicyDocument != nil {
					policyDetail := policyDetail{
						PolicyName:     types.StringValue(*getPolicyResponse.Body.Policy.PolicyName),
						PolicyDocument: types.StringValue(*getPolicyResponse.Body.DefaultPolicyVersion.PolicyDocument),
					}
					policyDetailsState = append(policyDetailsState, &policyDetail)
				}
			}
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(getPolicy, reconnectBackoff)
	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Failed to Read Policy.",
				err.Error(),
			),
		}
	}

	policyDetails := []attr.Value{}
	for _, policy := range policyDetailsState {
		policyDetails = append(policyDetails, types.ObjectValueMust(
			map[string]attr.Type{
				"policy_name":     types.StringType,
				"policy_document": types.StringType,
			},
			map[string]attr.Value{
				"policy_name":     types.StringValue(policy.PolicyName.ValueString()),
				"policy_document": types.StringValue(policy.PolicyDocument.ValueString()),
			},
		))
	}
	state.Policies = types.ListValueMust(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"policy_name":     types.StringType,
				"policy_document": types.StringType,
			},
		},
		policyDetails,
	)
	return nil
}

func (r *ramPolicyResource) removePolicy(state *ramPolicyResourceModel) diag.Diagnostics {
	data := make(map[string]string)

	removePolicy := func() error {
		for _, policies := range state.Policies.Elements() {
			runtime := &util.RuntimeOptions{}

			json.Unmarshal([]byte(policies.String()), &data)

			detachPolicyFromUserRequest := &alicloudRamClient.DetachPolicyFromUserRequest{
				PolicyType: tea.String("Custom"),
				PolicyName: tea.String(data["policy_name"]),
				UserName:   tea.String(state.UserName.ValueString()),
			}

			deletePolicyRequest := &alicloudRamClient.DeletePolicyRequest{
				PolicyName: tea.String(data["policy_name"]),
			}

			if _, err := r.client.DetachPolicyFromUserWithOptions(detachPolicyFromUserRequest, runtime); err != nil {
				handleAPIError(err)
			}

			if _, err := r.client.DeletePolicyWithOptions(deletePolicyRequest, runtime); err != nil {
				handleAPIError(err)
			}
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(removePolicy, reconnectBackoff)
	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Failed to Delete Policy",
				err.Error(),
			),
		}
	}

	return nil
}

func (r *ramPolicyResource) getPolicyDocument(plan *ramPolicyResourceModel) (finalPolicyDocument []string, err error) {
	policyName := ""
	currentLength := 0
	currentPolicyDocument := ""
	appendedPolicyDocument := make([]string, 0)
	finalPolicyDocument = make([]string, 0)

	var getPolicyResponse *alicloudRamClient.GetPolicyResponse

	for i, policy := range plan.AttachedPolicies.Elements() {
		policyName = policy.String()
		getPolicyRequest := &alicloudRamClient.GetPolicyRequest{
			PolicyType: tea.String("Custom"),
			PolicyName: tea.String(trimStringQuotes(policyName)),
		}

		getPolicy := func() error {
			runtime := &util.RuntimeOptions{}
			for {
				var err error
				getPolicyResponse, err = r.client.GetPolicyWithOptions(getPolicyRequest, runtime)
				if err != nil {
					if *getPolicyRequest.PolicyType == "System" {
						return backoff.Permanent(err)
					}
					if _, ok := err.(*tea.SDKError); ok {
						if *getPolicyRequest.PolicyType == "Custom" {
							*getPolicyRequest.PolicyType = "System"
							continue
						}
					} else {
						return err
					}
				} else {
					break
				}
			}

			return nil
		}

		reconnectBackoff := backoff.NewExponentialBackOff()
		reconnectBackoff.MaxElapsedTime = 30 * time.Second
		backoff.Retry(getPolicy, reconnectBackoff)

		if getPolicyResponse.Body != nil && getPolicyResponse.Body.DefaultPolicyVersion != nil {
			if getPolicyResponse.Body.DefaultPolicyVersion.PolicyDocument != nil {
				tempPolicyDocument := *getPolicyResponse.Body.DefaultPolicyVersion.PolicyDocument

				var data map[string]interface{}
				if err := json.Unmarshal([]byte(tempPolicyDocument), &data); err != nil {
					return nil, err
				}

				statementArr := data["Statement"].([]interface{})
				statementBytes, err := json.MarshalIndent(statementArr, "", "  ")
				if err != nil {
					return nil, err
				}

				removeSpaces := strings.ReplaceAll(string(statementBytes), " ", "")
				replacer := strings.NewReplacer("\n", "")
				removeParagraphs := replacer.Replace(removeSpaces)

				finalStatement := strings.Trim(removeParagraphs, "[]")

				currentLength += len(finalStatement)

				// Before further proceeding the current policy, we need to add a number of 30 to simulate the total length of completed policy to check whether it is already execeeded the max character length of 6144.
				// Number of 30 indicates the character length of neccessary policy keyword such as "Version" and "Statement" and some JSON symbols ({}, [])
				if (currentLength + 30) > maxLength {
					lastCommaIndex := strings.LastIndex(currentPolicyDocument, ",")
					if lastCommaIndex >= 0 {
						currentPolicyDocument = currentPolicyDocument[:lastCommaIndex] + currentPolicyDocument[lastCommaIndex+1:]
					}

					appendedPolicyDocument = append(appendedPolicyDocument, currentPolicyDocument)
					currentPolicyDocument = finalStatement + ","
					currentLength = len(finalStatement)
				} else {
					currentPolicyDocument += finalStatement + ","
				}

				if i == len(plan.AttachedPolicies.Elements())-1 && (currentLength+30) <= maxLength {
					lastCommaIndex := strings.LastIndex(currentPolicyDocument, ",")
					if lastCommaIndex >= 0 {
						currentPolicyDocument = currentPolicyDocument[:lastCommaIndex] + currentPolicyDocument[lastCommaIndex+1:]
					}
					appendedPolicyDocument = append(appendedPolicyDocument, currentPolicyDocument)
				}
			}
		} else {
			return nil, errors.New(fmt.Sprintf("The %v policy not found.", policyName))
		}
	}

	if len(appendedPolicyDocument) > 0 {
		for _, policy := range appendedPolicyDocument {
			finalPolicyDocument = append(finalPolicyDocument, fmt.Sprintf(`{"Version":"1","Statement":[%v]}`, policy))
		}
	}

	return finalPolicyDocument, nil
}

func (r *ramPolicyResource) attachPolicyToUser(state *ramPolicyResourceModel) (err error) {
	data := make(map[string]string)

	attachPolicyToUser := func() error {
		for _, policies := range state.Policies.Elements() {
			json.Unmarshal([]byte(policies.String()), &data)

			attachPolicyToUserRequest := &alicloudRamClient.AttachPolicyToUserRequest{
				PolicyType: tea.String("Custom"),
				PolicyName: tea.String(data["policy_name"]),
				UserName:   tea.String(state.UserName.ValueString()),
			}

			runtime := &util.RuntimeOptions{}
			if _, err := r.client.AttachPolicyToUserWithOptions(attachPolicyToUserRequest, runtime); err != nil {
				handleAPIError(err)
			}
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	return backoff.Retry(attachPolicyToUser, reconnectBackoff)
}

func handleAPIError(err error) error {
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
