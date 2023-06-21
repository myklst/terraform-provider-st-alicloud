package alicloud

import (
	"encoding/json"
	"strings"

	alicloudOpenapiClient "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// Convert the result for an array and returns a Json string
func convertListStringToJsonString(configured []string) string {
	if len(configured) < 1 {
		return ""
	}
	result := "["
	for i, v := range configured {
		if v == "" {
			continue
		}
		result += "\"" + v + "\""
		if i < len(configured)-1 {
			result += ","
		}
	}
	result += "]"
	return result
}

func convertJsonStringToListString(configured string) ([]string, error) {
	result := make([]string, 0)
	if err := json.Unmarshal([]byte(configured), &result); err != nil {
		return nil, err
	}

	return result, nil
}

func trimStringQuotes(input string) string {
	return strings.TrimPrefix(strings.TrimSuffix(input, "\""), "\"")
}

func initNewClient(providerConfig *alicloudOpenapiClient.Client, planConfig *clientConfig) (initClient bool, clientConfig *alicloudOpenapiClient.Config, diag diag.Diagnostics) {
	initClient = false
	clientConfig = &alicloudOpenapiClient.Config{}
	region := planConfig.Region.ValueString()
	accessKey := planConfig.AccessKey.ValueString()
	secretKey := planConfig.SecretKey.ValueString()

	if region != "" || accessKey != "" || secretKey != "" {
		initClient = true
	}

	if initClient {
		if region == "" {
			region = tea.StringValue(providerConfig.RegionId)
		}
		if accessKey == "" {
			clientAccessKey, err := providerConfig.Credential.GetAccessKeyId()
			if err != nil {
				diag.AddError(
					"Failed to retrieve client Access Key.",
					"This is an error in provider, please contact the provider developers.\n\n"+
						"Error: "+err.Error(),
				)
			} else {
				accessKey = tea.StringValue(clientAccessKey)
			}
		}
		if secretKey == "" {
			clientSecretKey, err := providerConfig.Credential.GetAccessKeySecret()
			if err != nil {
				diag.AddError(
					"Failed to retrieve client Secret Key.",
					"This is an error in provider, please contact the provider developers.\n\n"+
						"Error: "+err.Error(),
				)
			} else {
				secretKey = tea.StringValue(clientSecretKey)
			}
		}
		if diag.HasError() {
			return
		}

		clientConfig = &alicloudOpenapiClient.Config{
			RegionId:        &region,
			AccessKeyId:     &accessKey,
			AccessKeySecret: &secretKey,
		}
	}

	return
}
