resource "st-alicloud_ram_policy" "ram_policy" {
  attached_policies = ["AliyunECSFullAccess", "AliyunRAMFullAccess", "AliyunOSSFullAccess", "AliyunOTSFullAccess", ]
  user_name         = "devopsuser01"
}
