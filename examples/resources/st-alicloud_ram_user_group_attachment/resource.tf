resource "st-alicloud_ram_user_group_attachment" "ram_group" {
  group_name = "test-group"
  user_name  = "test-user"
}
