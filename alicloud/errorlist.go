package alicloud

const (
	ERR_CLOSE_DNS_SLB_FAILED  = "CloseDnsSlbFailed"
	ERR_DISABLE_DNS_SLB       = "DisableDNSSLB"
	ERR_ENABLE_DNS_SLB_FAILED = "EnableDnsSlbFailed"
	ERR_DNS_SYSTEM_BUSYNESS   = "DnsSystemBusyness"
	ERR_SERVICE_UNAVAILABLE   = "ServiceUnavailable"
	ERR_THROTTLING_USER       = "Throttling.User"
	ERR_THROTTLING_API        = "Throttling.API"
	ERR_THROTTLING            = "Throttling"
	ERR_UNKNOWN_ERROR         = "UnknownError"
	ERR_INTERNAL_ERROR        = "InternalError"
	ERR_BACKEND_TIMEOUT       = "D504TO"
)

func isAbleToRetry(errCode string) bool {
	switch errCode {
	case ERR_CLOSE_DNS_SLB_FAILED,
		ERR_DISABLE_DNS_SLB,
		ERR_ENABLE_DNS_SLB_FAILED,
		ERR_DNS_SYSTEM_BUSYNESS,
		ERR_SERVICE_UNAVAILABLE,
		ERR_THROTTLING_USER,
		ERR_THROTTLING_API,
		ERR_THROTTLING,
		ERR_UNKNOWN_ERROR,
		ERR_INTERNAL_ERROR:
		return true
	default:
		return false
	}
	// return false
}
