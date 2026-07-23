package alicloud

import "strings"

const (
	ERR_CLOSE_DNS_SLB_FAILED         = "closednsslbfailed"
	ERR_DISABLE_DNS_SLB              = "disablednsslb"
	ERR_ENABLE_DNS_SLB_FAILED        = "enablednsslbfailed"
	ERR_DNS_SYSTEM_BUSYNESS          = "dnssystembusyness"
	ERR_SERVICE_UNAVAILABLE          = "serviceunavailable"
	ERR_THROTTLING_USER              = "throttling.user"
	ERR_THROTTLING_API               = "throttling.api"
	ERR_THROTTLING                   = "throttling"
	ERR_UNKNOWN_ERROR                = "unknownerror"
	ERR_INTERNAL_ERROR               = "internalerror"
	ERR_BACKEND_TIMEOUT              = "d504to"
	ERR_INSTANCE_STATUS_NOT_SUPPORT  = "instancestatus.notsupport"
)

func isAbleToRetry(errCode string) bool {
	switch strings.ToLower(errCode) {
	case ERR_CLOSE_DNS_SLB_FAILED,
		ERR_DISABLE_DNS_SLB,
		ERR_ENABLE_DNS_SLB_FAILED,
		ERR_DNS_SYSTEM_BUSYNESS,
		ERR_SERVICE_UNAVAILABLE,
		ERR_THROTTLING_USER,
		ERR_THROTTLING_API,
		ERR_THROTTLING,
		ERR_UNKNOWN_ERROR,
		ERR_INTERNAL_ERROR,
		ERR_INSTANCE_STATUS_NOT_SUPPORT:
		return true
	default:
		return false
	}
	// return false
}
