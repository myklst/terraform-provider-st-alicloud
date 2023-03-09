resource "st-alicloud_alidns_gtm_instance" "test2" {
  payment_type      = "Subscription"
  alert_group       = ["test-network"]
  resource_group_id = "1234122123"
  ttl               = 60
  instance_name     = "test-gtm-instance"
  package_edition   = "standard"
  instance_type     = "intl"
  strategy_mode     = "GEO"
}
