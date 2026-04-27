package alicloud

// rpc_client.go — Alibaba Cloud RPC-style signing and HTTP client.
//
// This is a Go port of rpc_call.py and matches Alibaba Cloud RPC signing rules:
// - HMAC-SHA1 signing
// - Canonicalization of sorted parameters
// - percent-encoding matching Python's urllib.quote(safe="~")
// - Retry strategy with exponential backoff + jitter
//
// This client is intentionally low-level and used by ecd_client.go for higher-level ECD actions.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	maxRetries = 5
	baseDelay  = time.Second
	maxDelay   = 10 * time.Second
)

// percentEncode matches Python urllib.parse.quote(v, safe="~").
func percentEncode(v string) string {
	encoded := url.QueryEscape(v)
	encoded = strings.ReplaceAll(encoded, "+", "%20")  // ensure space = %20
	encoded = strings.ReplaceAll(encoded, "%7E", "~") // keep ~ unescaped
	return encoded
}

// canonicalize sorts params and produces the canonical string for signing.
func canonicalize(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, percentEncode(k)+"="+percentEncode(params[k]))
	}
	return strings.Join(parts, "&")
}

func shouldRetry(statusCode int, errText string) bool {
	if statusCode >= 500 {
		return true
	}
	if strings.Contains(errText, "Throttl") || strings.Contains(errText, "Rate") {
		return true
	}
	if strings.Contains(errText, "Timeout") {
		return true
	}
	return false
}

// signRPC computes HMAC-SHA1 signature exactly like Python version.
func signRPC(method, accessKeySecret string, params map[string]string) string {
	canonicalized := canonicalize(params)
	stringToSign := method + "&%2F&" + percentEncode(canonicalized)
	key := []byte(accessKeySecret + "&")

	mac := hmac.New(sha1.New, key)
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func uuidV4() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func sleepWithJitter(attempt int) {
	delay := time.Duration(math.Min(
		float64(baseDelay)*math.Pow(2, float64(attempt-1)),
		float64(maxDelay),
	))
	jitter := time.Duration(mathrand.Float64() * float64(500*time.Millisecond))
	time.Sleep(delay + jitter)
}

// RPCRequest is the exported version of rpcRequest().
// This function directly performs Alibaba Cloud RPC signing + call.
//
// endpoint example: "https://ecd.ap-southeast-3.aliyuncs.com/"
// method: "POST" or "GET"
// action: "CreateSimpleOfficeSite"
// version: "2020-09-30"
// region: "ap-southeast-3"
// bizParams: all request parameters for the API call (e.g., VpcId, OfficeSiteName, etc.)
func RPCRequest(
	endpoint, method, action, version, region,
	accessKey, secretKey string,
	bizParams map[string]string,
) (map[string]interface{}, error) {

	endpoint = strings.TrimRight(endpoint, "/") + "/"
	method = strings.ToUpper(method)

	params := map[string]string{
		"Action":           action,
		"Version":          version,
		"RegionId":         region,
		"Format":           "JSON",
		"AccessKeyId":      accessKey,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   uuidV4(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}

	for k, v := range bizParams {
		params[k] = v
	}

	params["Signature"] = signRPC(method, secretKey, params)

	httpClient := &http.Client{Timeout: 30 * time.Second}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var (
			resp *http.Response
			err  error
		)

		if method == "POST" {
			form := url.Values{}
			for k, v := range params {
				if v != "" {
					form.Set(k, v)
				}
			}
			resp, err = httpClient.Post(
				endpoint,
				"application/x-www-form-urlencoded",
				strings.NewReader(form.Encode()),
			)
		} else if method == "GET" {
			reqURL, _ := url.Parse(endpoint)
			q := url.Values{}
			for k, v := range params {
				q.Set(k, v)
			}
			reqURL.RawQuery = q.Encode()
			resp, err = httpClient.Get(reqURL.String())
		} else {
			return nil, fmt.Errorf("only GET/POST are supported")
		}

		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				sleepWithJitter(attempt)
			}
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode < 400 {
			var result map[string]interface{}
			if err := json.Unmarshal(body, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}
			return result, nil
		}

		errText := string(body)
		if !shouldRetry(resp.StatusCode, errText) {
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, errText)
		}

		lastErr = fmt.Errorf("retryable error %d: %s", resp.StatusCode, errText)
		if attempt < maxRetries {
			sleepWithJitter(attempt)
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}
