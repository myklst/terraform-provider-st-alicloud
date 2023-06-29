resource "st-alicloud_ram_policy" "ram_policy" {
  policy_name       = "test-policy"
  policy_type       = "Custom"
  attached_policies = ["AliyunECSFullAccess", "AliyunRAMFullAccess", "AliyunOSSFullAccess", "AliyunOTSFullAccess", ]
  user_name         = "devopsuser01"
}
