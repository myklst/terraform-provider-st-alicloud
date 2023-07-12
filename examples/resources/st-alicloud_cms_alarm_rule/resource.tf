resource "st-alicloud_cms_alarm_rule" "default" {
    rule_id = "test-rule-id"
    rule_name = "test-rule-name"
    namespace = "acs_emr" 
    metric_name = "test-metric-name"
    resources = "[  {\"resource-name\" : \"resource-value\" } ]"
    contact_groups = "test-contact-group"
    composite_expression = {
        expression_raw = "@test-metric-name[60].$Maximum>1"
        level = "critical"
        times = 5
    }
}