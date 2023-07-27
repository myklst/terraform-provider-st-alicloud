resource "st-alicloud_cms_composite_group_metric_rule" "default" {
  rule_name   = "test-rule-name"
  group_id    = "123123123"
  namespace   = "acs_emr" 
  metric_name = "yarn_cluster_availableVirtualCores"
  contact_groups = "test-contact-group"

  composite_expression = {
    expression_raw = "@yarn_cluster_availableVirtualCores[60].$Maximum / @yarn_cluster_totalVirtualCores[60].$Maximum <= 0.1"
    level          = "critical"
    times          = 3
  }
}

