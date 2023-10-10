resource "st-alicloud_emr_metric_auto_scaling_rules" "metric_auto_scaling" {
  cluster_id = "c-1234567890"
  max_nodes  = "10"
  min_nodes  = "1"

  scaling_rule {
    rule_name                 = "rule1"
    multi_metric_relationship = "Or"
    statistical_period        = 300
    evaluation_count          = 1
    scale_operation           = "SCALE_OUT"
    scaling_node_count        = 1
    cooldown_time             = 120

    metric_rule {
      metric_name         = "yarn_resourcemanager_queue_AvailableMBPercentage"
      comparison_operator = "LE"
      statistical_measure = "AVG"
      threshold           = "20"
    }

    metric_rule {
      metric_name         = "yarn_resourcemanager_queue_AvailableVCoresPercentage"
      comparison_operator = "LE"
      statistical_measure = "AVG"
      threshold           = "20"
    }
  }

  scaling_rule {
    rule_name                 = "rule2"
    multi_metric_relationship = "Or"
    statistical_period        = 300
    evaluation_count          = 1
    scale_operation           = "SCALE_IN"
    scaling_node_count        = 1
    cooldown_time             = 120

    metric_rule {
      metric_name         = "yarn_resourcemanager_queue_AvailableMBPercentage"
      comparison_operator = "GE"
      statistical_measure = "AVG"
      threshold           = "50"
    }

    metric_rule {
      metric_name         = "yarn_resourcemanager_queue_AvailableVCoresPercentage"
      comparison_operator = "GE"
      statistical_measure = "AVG"
      threshold           = "50"
    }
  }
}
