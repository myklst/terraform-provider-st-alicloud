resource "st-alicloud_aliadb_resource_group_bind_user" "bind_user" {
  dbcluster_id = "am-3ns9eg3ntm1g7y0m3"
  group_name   = "TEST"
  group_user   = "dts"
}
