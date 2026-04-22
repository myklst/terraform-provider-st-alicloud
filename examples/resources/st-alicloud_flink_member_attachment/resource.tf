resource "st-alicloud_flink_member_attachment" "member" {
  workspace_id = "ae07536b98154d"
  namespace    = "test-default"
  member_id    = "21xxxxxxxxxxxxxxxx"
  role         = "editor"
}
