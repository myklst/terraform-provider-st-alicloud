resource "st-alicloud_aliadb_scaling_plan" "scaling_plan" {
  db_cluster_id       = "amv-wz9509beptiz****"
  elastic_plan_name   = "adb_scale_plan"
  type                = "EXECUTOR"
  enabled             = true
  resource_group_name = "resource_group"
  target_size         = "32ACU"
  cron_expression     = "0 20 14 * * ?"
  start_time          = "2024-01-01T00:00:00Z"
  end_time            = "2025-12-31T23:59:59Z"
  auto_scale          = false
}
