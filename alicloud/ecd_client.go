package alicloud

import (
	"fmt"
	"strings"
)

// EcdClient wraps RPCRequest() with region + credentials retained.
type EcdClient struct {
	Region     string
	AccessKey  string
	SecretKey  string
	Endpoint   string
	APIVersion string
}

// NewEcdClient creates an Ecd client with explicit credentials.
func NewEcdClient(region, ak, sk string) *EcdClient {
	return &EcdClient{
		Region:     region,
		AccessKey:  ak,
		SecretKey:  sk,
		Endpoint:   fmt.Sprintf("https://ecd.%s.aliyuncs.com/", region),
		APIVersion: "2020-09-30",
	}
}

///////////////////////////////////////////////////////////////////////////////
//
//  High-level RPC operations used by the Terraform resource
//
///////////////////////////////////////////////////////////////////////////////

// DescribeOfficeSitesByVpc returns raw RPC response for DescribeOfficeSites with filter VpcId.
func (c *EcdClient) DescribeOfficeSitesByVpc(vpcId string) (map[string]interface{}, error) {
	body := map[string]string{
		"VpcId": vpcId,
	}

	resp, err := RPCRequest(
		c.Endpoint,
		"POST",
		"DescribeOfficeSites",
		c.APIVersion,
		c.Region,
		c.AccessKey,
		c.SecretKey,
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("DescribeOfficeSites failed: %w", err)
	}
	return resp, nil
}

// CreateSimpleOfficeSite creates a customized VPC SimpleOfficeSite.
func (c *EcdClient) CreateSimpleOfficeSite(params map[string]string) (string, error) {

	resp, err := RPCRequest(
		c.Endpoint,
		"POST",
		"CreateSimpleOfficeSite",
		c.APIVersion,
		c.Region,
		c.AccessKey,
		c.SecretKey,
		params,
	)

	if err != nil {
		return "", fmt.Errorf("CreateSimpleOfficeSite failed: %w", err)
	}

	// OfficeSiteId can appear in two different places...
	if id, ok := resp["OfficeSiteId"].(string); ok && id != "" {
		return id, nil
	}

	if result, ok := resp["Result"].(map[string]interface{}); ok {
		if id, ok2 := result["OfficeSiteId"].(string); ok2 && id != "" {
			return id, nil
		}
	}

	return "", fmt.Errorf("CreateSimpleOfficeSite: OfficeSiteId missing in response %v", resp)
}

// DeleteOfficeSite performs DeleteOfficeSites RPC call.
func (c *EcdClient) DeleteOfficeSite(id string) error {
	body := map[string]string{
		"OfficeSiteId.1": id,
	}

	_, err := RPCRequest(
		c.Endpoint,
		"POST",
		"DeleteOfficeSites",
		c.APIVersion,
		c.Region,
		c.AccessKey,
		c.SecretKey,
		body,
	)

	if err != nil {
		return fmt.Errorf("DeleteOfficeSites failed: %w", err)
	}
	return nil
}

// DescribeOfficeSiteById returns the office site map for the given OfficeSiteId, or nil if not found.
func (c *EcdClient) DescribeOfficeSiteById(id string) (map[string]interface{}, error) {
	body := map[string]string{
		"OfficeSiteId.1": id,
	}

	resp, err := RPCRequest(
		c.Endpoint,
		"POST",
		"DescribeOfficeSites",
		c.APIVersion,
		c.Region,
		c.AccessKey,
		c.SecretKey,
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("DescribeOfficeSites failed: %w", err)
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
	for k, v := range params {
		body[k] = v
	}
	body["OfficeSiteId"] = id

	_, err := RPCRequest(
		c.Endpoint,
		"POST",
		"ModifyOfficeSiteAttribute",
		c.APIVersion,
		c.Region,
		c.AccessKey,
		c.SecretKey,
		body,
	)
	if err != nil {
		return fmt.Errorf("ModifyOfficeSiteAttribute failed: %w", err)
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////
//
//  Helper functions for DescribeOfficeSites responses
//
///////////////////////////////////////////////////////////////////////////////

// ExtractOfficeSites returns []map[string]interface{} from raw RPC response.
func ExtractOfficeSites(resp map[string]interface{}) []map[string]interface{} {
	if resp == nil {
		return nil
	}

	rawOfficeSites, ok := resp["OfficeSites"]
	if !ok {
		return nil
	}

	switch v := rawOfficeSites.(type) {
	case map[string]interface{}:
		if list, ok := v["OfficeSite"].([]interface{}); ok {
			return convertToMapList(list)
		}
	case []interface{}:
		return convertToMapList(v)
	}

	return nil
}

// ExtractOfficeSiteIdByVpc returns OfficeSiteId for sites matching VpcId.
func ExtractOfficeSiteIdByVpc(resp map[string]interface{}, vpcId string) (string, bool) {
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

func convertToMapList(list []interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}
