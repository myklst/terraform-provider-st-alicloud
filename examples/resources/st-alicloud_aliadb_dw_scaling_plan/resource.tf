resource "st-alicloud_aliadb_dw_scaling_plan" "monthly_plan" {
  db_cluster_id               = "am-bp1xxxxxxxx"
  elastic_plan_name           = "adb_scale_plan"
  elastic_plan_time_start     = "01:00:00"
  elastic_plan_time_end       = "05:00:00"
  elastic_plan_type           = "executor"
  elastic_plan_start_day      = "2026-07-01"
  elastic_plan_end_day        = "2027-07-01"
  elastic_plan_enable         = true
  elastic_plan_node_num       = 5
  elastic_plan_monthly_repeat = "28,29,30,31"
}
