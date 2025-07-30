package alicloud

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	alicloudRamClient "github.com/alibabacloud-go/ram-20150501/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

const (
	// Number of 30 indicates the character length of neccessary policy keyword
	// such as "Version" and "Statement" and some JSON symbols ({}, []).
	policyKeywordLength = 30
	policyMaxLength     = 6144
)

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
	UserName               types.String    `tfsdk:"user_name"`
	AttachedPolicies       types.List      `tfsdk:"attached_policies"`
	AttachedPoliciesDetail []*policyDetail `tfsdk:"attached_policies_detail"`
	CombinedPolicesDetail  []*policyDetail `tfsdk:"combined_policies_detail"`
	Policies               []*policyDetail `tfsdk:"policies"` // TODO: Remove in next version when 'Policies' is moved to CombinedPoliciesDetail.
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
		Description: "Provides a RAM Policy resource that manages policy content " +
			"exceeding character limits by splitting it into smaller segments. " +
			"These segments are combined to form a complete policy attached to " +
			"the user. However, the policy that exceed the maximum length of a " +
			"policy, they will be attached directly to the user.",
		Attributes: map[string]schema.Attribute{
			"user_name": schema.StringAttribute{
				Description: "The name of the RAM user that attached to the policy.",
				Required:    true,
			},
			"attached_policies": schema.ListAttribute{
				Description: "The RAM policies to attach to the user.",
				Required:    true,
				ElementType: types.StringType,
			},
			"attached_policies_detail": schema.ListNestedAttribute{
				Description: "A list of policies. Used to compare whether policy has been changed outside of Terraform",
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
			"combined_policies_detail": schema.ListNestedAttribute{
				Description: "A list of combined policies that are attached to users.",
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
			// NOTE: Avoid using 'policies' in new implementations; use 'CombinedPolicies' instead.
			// TODO: Remove in next version when 'Policies' is moved to CombinedPoliciesDetail.
			"policies": schema.ListNestedAttribute{
				Description: "[Deprecated] A list of policies.",
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
	getPlanDiags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	combinedPolicies, attachedPolicies, errors := r.createPolicy(ctx, plan)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		"[API ERROR] Failed to Create the Policy.",
		errors,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	state := &ramPolicyResourceModel{}
	state.UserName = plan.UserName
	state.AttachedPolicies = plan.AttachedPolicies
	state.AttachedPoliciesDetail = attachedPolicies
	state.CombinedPolicesDetail = combinedPolicies

	err := r.attachPolicyToUser(state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		"[API ERROR] Failed to Attach Policy to User.",
		[]error{err},
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create policy are not expected to have not found warning.
	readCombinedPolicyNotExistErr, readCombinedPolicyErr := r.readCombinedPolicy(state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Policy Not Found!", state.UserName),
		readCombinedPolicyNotExistErr,
		"",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Unexpected Error!", state.UserName),
		readCombinedPolicyErr,
		"",
	)
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

	// NOTE: Avoid using 'policies' in new implementations; use 'CombinedPolicies' instead.
	// TODO: Remove in next version when 'Policies' is moved to CombinedPoliciesDetail.
	if len(state.CombinedPolicesDetail) == 0 && len(state.Policies) != 0 {
		state.CombinedPolicesDetail = state.Policies
		state.Policies = nil
	}

	// This state will be using to compare with the current state.
	var oriState *ramPolicyResourceModel
	getOriStateDiags := req.State.Get(ctx, &oriState)
	resp.Diagnostics.Append(getOriStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// NOTE: Avoid using 'policies' in new implementations; use 'CombinedPolicies' instead.
	// TODO: Remove in next version when 'Policies' is moved to CombinedPoliciesDetail.
	if len(oriState.CombinedPolicesDetail) == 0 && len(oriState.Policies) != 0 {
		oriState.CombinedPolicesDetail = oriState.Policies
		oriState.Policies = nil
	}

	readCombinedPolicyNotExistErr, readCombinedPolicyErr := r.readCombinedPolicy(state)
	addDiagnostics(
		&resp.Diagnostics,
		"warning",
		fmt.Sprintf("[API WARNING] Failed to Read Combined Policies for %v: Policy Not Found!", state.UserName),
		readCombinedPolicyNotExistErr,
		"The combined policies may be deleted due to human mistake or API error, will trigger update to recreate the combined policy:",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Unexpected Error!", state.UserName),
		readCombinedPolicyErr,
		"",
	)

	// Set state so that Terraform will trigger update if there are changes in state.
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.WarningsCount() > 0 || resp.Diagnostics.HasError() {
		return
	}

	// If the attached policy not found, it should return warning instead of error
	// because there is no ways to get plan configuration in Read() function to
	// indicate user had removed the non existed policies from the input.
	readAttachedPolicyNotExistErr, readAttachedPolicyErr := r.readAttachedPolicy(state)
	addDiagnostics(
		&resp.Diagnostics,
		"warning",
		fmt.Sprintf("[API WARNING] Failed to Read Attached Policies for %v: Policy Not Found!", state.UserName),
		readAttachedPolicyNotExistErr,
		"The policy that will be used to combine policies had been removed on AliCloud, next apply with update will prompt error:",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Attached Policies for %v: Unexpected Error!", state.UserName),
		readAttachedPolicyErr,
		"",
	)

	// Set state so that Terraform will trigger update if there are changes in state.
	setStateDiags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.WarningsCount() > 0 || resp.Diagnostics.HasError() {
		return
	}

	compareAttachedPoliciesErr := r.checkPoliciesDrift(state, oriState)
	addDiagnostics(
		&resp.Diagnostics,
		"warning",
		fmt.Sprintf("[API WARNING] Policy Drift Detected for %v.", state.UserName),
		[]error{compareAttachedPoliciesErr},
		"This resource will be updated in the next terraform apply.",
	)

	setStateDiags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ramPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state *ramPolicyResourceModel
	getPlanDiags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// NOTE: Avoid using 'policies' in new implementations; use 'CombinedPolicies' instead.
	// TODO: Remove in next version when 'Policies' is moved to CombinedPoliciesDetail.
	if len(state.CombinedPolicesDetail) == 0 && len(state.Policies) != 0 {
		state.CombinedPolicesDetail = state.Policies
		state.Policies = nil
	}

	// Make sure each of the attached policies are exist before removing the combined
	// policies.
	readAttachedPolicyNotExistErr, readAttachedPolicyErr := r.readAttachedPolicy(plan)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Attached Policies for %v: Policy Not Found!", state.UserName),
		readAttachedPolicyNotExistErr,
		"The policy that will be used to combine policies had been removed on AliCloud:",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Attached Policies for %v: Unexpected Error!", state.UserName),
		readAttachedPolicyErr,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	removePolicyDiags := r.removePolicy(state)
	resp.Diagnostics.Append(removePolicyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.CombinedPolicesDetail = nil
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	combinedPolicies, attachedPolicies, errors := r.createPolicy(ctx, plan)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		"[API ERROR] Failed to Create the Policy.",
		errors,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	state.UserName = plan.UserName
	state.AttachedPolicies = plan.AttachedPolicies
	state.AttachedPoliciesDetail = attachedPolicies
	state.CombinedPolicesDetail = combinedPolicies

	err := r.attachPolicyToUser(state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		"[API ERROR] Failed to Attach Policy to User.",
		[]error{err},
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create policy are not expected to have not found warning.
	readCombinedPolicyNotExistErr, readCombinedPolicyErr := r.readCombinedPolicy(state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Policy Not Found!", state.UserName),
		readCombinedPolicyNotExistErr,
		"",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Unexpected Error!", state.UserName),
		readCombinedPolicyErr,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	setStateDiags = resp.State.Set(ctx, &state)
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

	// NOTE: Avoid using 'policies' in new implementations; use 'CombinedPolicies' instead.
	// TODO: Remove this data transfer and 'policies' when said variable is no longer used.
	if len(state.CombinedPolicesDetail) == 0 && len(state.Policies) != 0 {
		state.CombinedPolicesDetail = state.Policies
		state.Policies = nil
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
	for _, policyName := range policyNames {
		getPolicy := func() error {
			runtime := &util.RuntimeOptions{}
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
			return nil
		}
		reconnectBackoff := backoff.NewExponentialBackOff()
		reconnectBackoff.MaxElapsedTime = 30 * time.Second
		if err = backoff.Retry(getPolicy, reconnectBackoff); err != nil {
			return
		}
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

// createPolicy will create the combined policy and return the attached policies
// details to be saved in state for comparing in Read() function.
//
// Parameters:
//   - ctx: Context.
//   - plan: Terraform plan configurations.
//
// Returns:
//   - combinedPoliciesDetail: The combined policies detail to be recorded in state file.
//   - attachedPoliciesDetail: The attached policies detail to be recorded in state file.
//   - errList: List of errors, return nil if no errors.
func (r *ramPolicyResource) createPolicy(ctx context.Context, plan *ramPolicyResourceModel) (combinedPoliciesDetail []*policyDetail, attachedPoliciesDetail []*policyDetail, errList []error) {
	var policies []string
	plan.AttachedPolicies.ElementsAs(ctx, &policies, false)
	combinedPolicyDocuments, excludedPolicies, attachedPoliciesDetail, errList := r.combinePolicyDocument(policies)
	if errList != nil {
		return nil, nil, errList
	}

	for i, policy := range combinedPolicyDocuments {
		policyName := fmt.Sprintf("%s-%d", plan.UserName.ValueString(), i+1)

		createPolicy := func() error {
			runtime := &util.RuntimeOptions{}
			createPolicyRequest := &alicloudRamClient.CreatePolicyRequest{
				PolicyName:     tea.String(policyName),
				PolicyDocument: tea.String(policy),
			}

			if _, err := r.client.CreatePolicyWithOptions(createPolicyRequest, runtime); err != nil {
				return handleAPIError(err)
			}
			return nil
		}
		reconnectBackoff := backoff.NewExponentialBackOff()
		reconnectBackoff.MaxElapsedTime = 30 * time.Second
		if err := backoff.Retry(createPolicy, reconnectBackoff); err != nil {
			return nil, nil, []error{err}
		}

		combinedPoliciesDetail = append(combinedPoliciesDetail, &policyDetail{
			PolicyName:     types.StringValue(policyName),
			PolicyDocument: types.StringValue(policy),
		})
	}

	// These policies will be attached directly to the user since splitting the
	// policy "statement" will be hitting the limitation of "maximum number of
	// attached policies" easily.
	combinedPoliciesDetail = slices.Concat(combinedPoliciesDetail, excludedPolicies)
	return combinedPoliciesDetail, attachedPoliciesDetail, nil
}

// combinePolicyDocument combine the policy with custom logic.
//
// Parameters:
//   - attachedPolicies: List of user attached policies to be combined.
//
// Returns:
//   - combinedPolicyDocument: The completed policy document after combining attached policies.
//   - excludedPolicies: If the target policy exceeds maximum length, then do not combine the policy and return as excludedPolicies.
//   - attachedPoliciesDetail: The attached policies detail to be recorded in state file.
//   - errList: List of errors, return nil if no errors.
func (r *ramPolicyResource) combinePolicyDocument(attachedPolicies []string) (combinedPolicyDocument []string, excludedPolicies []*policyDetail, attachedPoliciesDetail []*policyDetail, errList []error) {
	attachedPoliciesDetail, notExistErrList, unexpectedErrList := r.fetchPolicies(attachedPolicies, []string{"Custom", "System"})

	errList = append(errList, notExistErrList...)
	errList = append(errList, unexpectedErrList...)

	if len(errList) != 0 {
		return nil, nil, nil, errList
	}

	currentLength := 0
	currentPolicyStatement := ""
	appendedPolicyStatement := make([]string, 0)

	for _, attachedPolicy := range attachedPoliciesDetail {
		tempPolicyDocument := attachedPolicy.PolicyDocument.ValueString()
		// If the policy itself have more than 6144 characters, then skip the combine
		// policy part since splitting the policy "statement" will be hitting the
		// limitation of "maximum number of attached policies" easily.
		if len(tempPolicyDocument) > policyMaxLength {
			excludedPolicies = append(excludedPolicies, &policyDetail{
				PolicyName:     attachedPolicy.PolicyName,
				PolicyDocument: types.StringValue(tempPolicyDocument),
			})
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(tempPolicyDocument), &data); err != nil {
			errList = append(errList, err)
			return nil, nil, nil, errList
		}

		statementBytes, err := json.Marshal(data["Statement"])
		if err != nil {
			errList = append(errList, err)
			return nil, nil, nil, errList
		}

		finalStatement := strings.Trim(string(statementBytes), "[]")
		currentLength += len(finalStatement)

		// Before further proceeding the current policy, we need to add a number
		// of 'policyKeywordLength' to simulate the total length of completed
		// policy to check whether it is already execeeded the max character
		// length of 6144.
		if (currentLength + policyKeywordLength) > policyMaxLength {
			currentPolicyStatement = strings.TrimSuffix(currentPolicyStatement, ",")
			appendedPolicyStatement = append(appendedPolicyStatement, currentPolicyStatement)
			currentPolicyStatement = finalStatement + ","
			currentLength = len(finalStatement)
		} else {
			currentPolicyStatement += finalStatement + ","
		}
	}

	if len(currentPolicyStatement) > 0 {
		currentPolicyStatement = strings.TrimSuffix(currentPolicyStatement, ",")
		appendedPolicyStatement = append(appendedPolicyStatement, currentPolicyStatement)
	}

	for _, policyStatement := range appendedPolicyStatement {
		combinedPolicyDocument = append(combinedPolicyDocument, fmt.Sprintf(`{"Version":"1","Statement":[%v]}`, policyStatement))
	}

	return combinedPolicyDocument, excludedPolicies, attachedPoliciesDetail, nil
}

// readCombinedPolicy will read the combined policy details.
//
// Parameters:
//   - state: The state configurations, it will directly update the value of the struct since it is a pointer.
//
// Returns:
//   - notExistError: List of allowed not exist errors to be used as warning messages instead, return nil if no errors.
//   - unexpectedError: List of unexpected errors to be used as normal error messages, return nil if no errors.
func (r *ramPolicyResource) readCombinedPolicy(state *ramPolicyResourceModel) (notExistErrs, unexpectedErrs []error) {
	var policiesName []string
	for _, policy := range state.CombinedPolicesDetail {
		policiesName = append(policiesName, policy.PolicyName.ValueString())
	}

	policyDetails, notExistErrs, unexpectedErrs := r.fetchPolicies(policiesName, []string{"Custom"})
	if len(unexpectedErrs) > 0 {
		return nil, unexpectedErrs
	}

	// If the combined policies not found from AliCloud, that it might be deleted
	// from outside Terraform. Set the state to Unknown to trigger state changes
	// and Update() function.
	if len(notExistErrs) > 0 {
		// This is to ensure Update() is called.
		state.AttachedPolicies = types.ListNull(types.StringType)
	}

	state.CombinedPolicesDetail = policyDetails
	return notExistErrs, nil
}

// readAttachedPolicy will read the attached policy details.
//
// Parameters:
//   - state: The state configurations, it will directly update the value of the struct since it is a pointer.
//
// Returns:
//   - notExistError: List of allowed not exist errors to be used as warning messages instead, return nil if no errors.
//   - unexpectedError: List of unexpected errors to be used as normal error messages, return nil if no errors.
func (r *ramPolicyResource) readAttachedPolicy(state *ramPolicyResourceModel) (notExistErrs, unexpectedErrs []error) {
	var policiesName []string
	for _, policyName := range state.AttachedPolicies.Elements() {
		policiesName = append(policiesName, strings.Trim(policyName.String(), "\""))
	}

	policyDetails, notExistErrs, unexpectedErrs := r.fetchPolicies(policiesName, []string{"Custom", "System"})
	if len(unexpectedErrs) > 0 {
		return nil, unexpectedErrs
	}

	// If the combined policies not found from AliCloud, that it might be deleted
	// from outside Terraform. Set the state to Unknown to trigger state changes
	// and Update() function.
	if len(notExistErrs) > 0 {
		// This is to ensure Update() is called.
		state.AttachedPolicies = types.ListNull(types.StringType)
	}

	state.AttachedPoliciesDetail = policyDetails
	return notExistErrs, nil
}

// fetchPolicies retrieve policy document through AliCloud SDK with backoff retry.
//
// Parameters:
//   - policiesName: List of RAM policies name.
//   - policyTypes: List of RAM policy types to retrieve.
//
// Returns:
//   - policiesDetail: List of retrieved policies detail.
//   - notExistError: List of allowed not exist errors to be used as warning messages instead, return empty list if no errors.
//   - unexpectedError: List of unexpected errors to be used as normal error messages, return empty list if no errors.
func (r *ramPolicyResource) fetchPolicies(policiesName []string, policyTypes []string) (policiesDetail []*policyDetail, notExistError, unexpectedError []error) {
	for _, attachedPolicy := range policiesName {
		getPolicyResponse := &alicloudRamClient.GetPolicyResponse{}
		var err error

		getPolicy := func() error {
			runtime := &util.RuntimeOptions{}

			for _, ramPolicyType := range policyTypes {
				getPolicyRequest := &alicloudRamClient.GetPolicyRequest{
					PolicyName: tea.String(strings.Trim(attachedPolicy, "\"")),
					PolicyType: tea.String(ramPolicyType),
				}
				getPolicyResponse, err = r.client.GetPolicyWithOptions(getPolicyRequest, runtime)
				if err != nil {
					// If policy not found, then continue to next policy type.
					if tea.StringValue(err.(*tea.SDKError).Code) == "EntityNotExist.Policy" {
						continue
					} else {
						return handleAPIError(err)
					}
				}
				return nil
			}
			return nil
		}

		reconnectBackoff := backoff.NewExponentialBackOff()
		reconnectBackoff.MaxElapsedTime = 30 * time.Second
		backoff.Retry(getPolicy, reconnectBackoff)

		// Handle permanent error returned from API.
		if err != nil {
			switch tea.StringValue(err.(*tea.SDKError).Code) {
			// The error handling here is different from the one in backoff retry
			// function. The error handling here represent the RAM policy is not
			// found in all policy types.
			case "EntityNotExist.Policy":
				notExistError = append(notExistError, err)
			default:
				unexpectedError = append(unexpectedError, err)
			}
		} else {
			policiesDetail = append(policiesDetail, &policyDetail{
				PolicyName:     types.StringValue(*getPolicyResponse.Body.Policy.PolicyName),
				PolicyDocument: types.StringValue(*getPolicyResponse.Body.DefaultPolicyVersion.PolicyDocument),
			})
		}
	}

	return
}

// checkPoliciesDrift compare the recorded AttachedPoliciesDetail documents with
// the latest RAM policy documents on AliCloud, and trigger Update() if policy
// drift is detected.
//
// Parameters:
//   - newState: New attached policy details that returned from AliCloud SDK.
//   - oriState: Original policy details that are recorded in Terraform state.
//
// Returns:
//   - error: The policy drifting error.
func (r *ramPolicyResource) checkPoliciesDrift(newState, oriState *ramPolicyResourceModel) error {
	var driftedPolicies []string

	for _, oldPolicyDetailState := range oriState.AttachedPoliciesDetail {
		for _, currPolicyDetailState := range newState.AttachedPoliciesDetail {
			if oldPolicyDetailState.PolicyName.String() == currPolicyDetailState.PolicyName.String() {
				if oldPolicyDetailState.PolicyDocument.String() != currPolicyDetailState.PolicyDocument.String() {
					driftedPolicies = append(driftedPolicies, oldPolicyDetailState.PolicyName.String())
				}
				break
			}
		}
	}

	if len(driftedPolicies) > 0 {
		// Set the state to trigger an update.
		newState.AttachedPolicies = types.ListNull(types.StringType)

		return fmt.Errorf(
			"the following policies documents had been changed since combining policies: [%s]",
			strings.Join(driftedPolicies, ", "),
		)
	}

	return nil
}

// removePolicy will detach and delete the combined policies from user.
//
// Parameters:
//   - state: The recorded state configurations.
func (r *ramPolicyResource) removePolicy(state *ramPolicyResourceModel) diag.Diagnostics {
	for _, combinedPolicy := range state.CombinedPolicesDetail {
		removePolicy := func() error {
			runtime := &util.RuntimeOptions{}
			detachPolicyFromUserRequest := &alicloudRamClient.DetachPolicyFromUserRequest{
				PolicyType: tea.String("Custom"),
				PolicyName: tea.String(combinedPolicy.PolicyName.ValueString()),
				UserName:   tea.String(state.UserName.ValueString()),
			}
			deletePolicyRequest := &alicloudRamClient.DeletePolicyRequest{
				PolicyName: tea.String(combinedPolicy.PolicyName.ValueString()),
			}
			if _, err := r.client.DetachPolicyFromUserWithOptions(detachPolicyFromUserRequest, runtime); err != nil {
				// Ignore error where the policy is not attached
				// to the user as it is intented to detach the
				// policy from user.
				if tea.StringValue(err.(*tea.SDKError).Code) != "EntityNotExist.User.Policy" {
					return handleAPIError(err)
				}
			}
			if _, err := r.client.DeletePolicyWithOptions(deletePolicyRequest, runtime); err != nil {
				// Ignore error where the policy had been deleted
				// as it is intended to delete the RAM policy.
				if tea.StringValue(err.(*tea.SDKError).Code) != "EntityNotExist.Policy" {
					return handleAPIError(err)
				}
			}
			return nil
		}
		reconnectBackoff := backoff.NewExponentialBackOff()
		reconnectBackoff.MaxElapsedTime = 30 * time.Second
		if err := backoff.Retry(removePolicy, reconnectBackoff); err != nil {
			return diag.Diagnostics{
				diag.NewErrorDiagnostic(
					"[API ERROR] Failed to Delete Policy",
					err.Error(),
				),
			}
		}
	}
	return nil
}

// attachPolicyToUser attach the RAM policy to user through AliCloud SDK.
//
// Parameters:
//   - state: The recorded state configurations.
//
// Returns:
//   - err: Error.
func (r *ramPolicyResource) attachPolicyToUser(state *ramPolicyResourceModel) (err error) {
	attachPolicyToUser := func() error {
		for _, combinedPolicy := range state.CombinedPolicesDetail {
			attachPolicyToUserRequest := &alicloudRamClient.AttachPolicyToUserRequest{
				PolicyType: tea.String("Custom"),
				PolicyName: tea.String(combinedPolicy.PolicyName.ValueString()),
				UserName:   tea.String(state.UserName.ValueString()),
			}

			runtime := &util.RuntimeOptions{}
			if _, err := r.client.AttachPolicyToUserWithOptions(attachPolicyToUserRequest, runtime); err != nil {
				return handleAPIError(err)
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

func addDiagnostics(diags *diag.Diagnostics, severity string, title string, errors []error, extraMessage string) {
	var combinedMessages string
	validErrors := 0

	for _, err := range errors {
		if err != nil {
			combinedMessages += fmt.Sprintf("%v\n", err)
			validErrors++
		}
	}

	if validErrors == 0 {
		return
	}

	var message string
	if extraMessage != "" {
		message = fmt.Sprintf("%s\n%s", extraMessage, combinedMessages)
	} else {
		message = combinedMessages
	}

	switch severity {
	case "warning":
		diags.AddWarning(title, message)
	case "error":
		diags.AddError(title, message)
	default:
		// Handle unknown severity if needed
	}
}
