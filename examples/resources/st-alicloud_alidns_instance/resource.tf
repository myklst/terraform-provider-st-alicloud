resource "st-alicloud_alidns_instance" "dns_instance" {
	domain_numbers = 1
	payment_type   = "Subscription"
	period         = 1
	renewal_status = "ManualRenewal"
	version_code   = "version_enterprise_basic"
	dns_security   = "no"
}
