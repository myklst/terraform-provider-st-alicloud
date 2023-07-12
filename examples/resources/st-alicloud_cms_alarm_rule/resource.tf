resource "st-alicloud_cms_alarm_rule" "alarm_rule" {
  rule_name      = "test-rule-name"
  namespace      = "acs_emr"
  metric_name    = "test-metric-name"
  contact_groups = "test-contact-group"
  composite_expression = {
    expression_raw = "@test-metric-name[60].$Maximum>1"
    level          = "critical"
    times          = 5
  }

  resources {
    resource_category = "test-resource-1"
    resource_value    = "test-resource-value-1"
  }
  resources {
    resource_category = "test-resource-2"
    resource_value    = "test-resource-value-2"
  }
}
