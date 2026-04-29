// Package alicloud provides Terraform resources for Alibaba Cloud services.
//
// sige_ecd_client.go — Custom ECD API client built on darabonba-openapi
// transport.
//
// The customized simple office site is currently only available to Alibaba
// Cloud accounts that have been explicitly whitelisted. The official SDK does
// not expose all parameters required by whitelisted accounts,
// VpcType = "customized" is not accepted by the generated SDK structs.
// Calling the RPC endpoint directly via darabonba-openapi gives us full control
// over request parameters without forking the SDK. Once AliCloud publicly
// releases these parameters in the SDK, this file can be replaced by the
// corresponding official client calls.
package alicloud

import (
	"fmt"
	"maps"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	openapiutil "github.com/alibabacloud-go/openapi-util/service"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

const ecdAPIVersion = "2020-09-30"

// EcdClient wraps darabonba-openapi Client for direct ECD RPC calls.
type EcdClient struct {
	client *openapi.Client
}

// NewEcdClient creates an EcdClient backed by the darabonba-openapi transport.
func NewEcdClient(region, ak, sk string) *EcdClient {
	config := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		RegionId:        tea.String(region),
		Endpoint:        tea.String(fmt.Sprintf("ecd.%s.aliyuncs.com", region)),
	}
	client, _ := openapi.NewClient(config)
	return &EcdClient{client: client}
}

// callRPC is a shared helper that dispatches a single ECD RPC action.
func (c *EcdClient) callRPC(action string, params map[string]string) (map[string]any, error) {
	apiParams := &openapi.Params{
		Action:      tea.String(action),
		Version:     tea.String(ecdAPIVersion),
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

	runtime := &util.RuntimeOptions{
		Autoretry:   tea.Bool(true),
		MaxAttempts: tea.Int(5),
	}

	resp, err := c.client.CallApi(apiParams, request, runtime)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", action, err)
	}

	if body, ok := resp["body"].(map[string]any); ok {
		return body, nil
	}
	return resp, nil
}

///////////////////////////////////////////////////////////////////////////////
//
//  High-level RPC operations used by the Terraform resource
//
///////////////////////////////////////////////////////////////////////////////

// DescribeOfficeSitesByVpc returns raw RPC response for DescribeOfficeSites with filter VpcId.
func (c *EcdClient) DescribeOfficeSitesByVpc(vpcId string) (map[string]any, error) {
	return c.callRPC("DescribeOfficeSites", map[string]string{
		"VpcId": vpcId,
	})
}

// CreateSimpleOfficeSite creates a VPC SimpleOfficeSite.
func (c *EcdClient) CreateSimpleOfficeSite(params map[string]string) (string, error) {
	resp, err := c.callRPC("CreateSimpleOfficeSite", params)
	if err != nil {
		return "", err
	}

	// OfficeSiteId can appear in two different places...
	if id, ok := resp["OfficeSiteId"].(string); ok && id != "" {
		return id, nil
	}

	if result, ok := resp["Result"].(map[string]any); ok {
		if id, ok2 := result["OfficeSiteId"].(string); ok2 && id != "" {
			return id, nil
		}
	}

	return "", fmt.Errorf("CreateSimpleOfficeSite: OfficeSiteId missing in response %v", resp)
}

// DeleteOfficeSite performs DeleteOfficeSites RPC call.
func (c *EcdClient) DeleteOfficeSite(id string) error {
	_, err := c.callRPC("DeleteOfficeSites", map[string]string{
		"OfficeSiteId.1": id,
	})
	return err
}

// DescribeOfficeSiteById returns the office site map for the given OfficeSiteId, or nil if not found.
func (c *EcdClient) DescribeOfficeSiteById(id string) (map[string]any, error) {
	resp, err := c.callRPC("DescribeOfficeSites", map[string]string{
		"OfficeSiteId.1": id,
	})
	if err != nil {
		return nil, err
	}

	sites := ExtractOfficeSites(resp)
	if len(sites) == 0 {
		return nil, nil
	}
	return sites[0], nil
}

// ModifyOfficeSiteAttribute updates mutable attributes of an office site.
// Supported params: OfficeSiteName, DesktopAccessType, EnableAdminAccess, etc.
func (c *EcdClient) ModifyOfficeSiteAttribute(id string, params map[string]string) error {
	body := make(map[string]string, len(params)+1)
	maps.Copy(body, params)
	body["OfficeSiteId"] = id

	_, err := c.callRPC("ModifyOfficeSiteAttribute", body)
	return err
}

///////////////////////////////////////////////////////////////////////////////
//
//  Helper functions for DescribeOfficeSites responses
//
///////////////////////////////////////////////////////////////////////////////

// ExtractOfficeSites returns []map[string]any from raw RPC response.
func ExtractOfficeSites(resp map[string]any) []map[string]any {
	if resp == nil {
		return nil
	}

	rawOfficeSites, ok := resp["OfficeSites"]
	if !ok {
		return nil
	}

	switch v := rawOfficeSites.(type) {
	case map[string]any:
		if list, ok := v["OfficeSite"].([]any); ok {
			return convertToMapList(list)
		}
	case []any:
		return convertToMapList(v)
	}

	return nil
}

// ExtractOfficeSiteIdByVpc returns OfficeSiteId for sites matching VpcId.
func ExtractOfficeSiteIdByVpc(resp map[string]any, vpcId string) (string, bool) {
	sites := ExtractOfficeSites(resp)
	if len(sites) == 0 {
		return "", false
	}

	for _, site := range sites {
		if strings.EqualFold(site["VpcId"].(string), vpcId) {
			if id, ok := site["OfficeSiteId"].(string); ok {
				return id, true
			}
		}
	}
	return "", false
}

func convertToMapList(list []any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
