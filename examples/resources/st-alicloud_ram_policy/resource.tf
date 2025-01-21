resource "st-alicloud_ram_policy" "ram_policy" {
  user_name = "devopsuser01"
  attached_policies = ["AliyunRAMReadOnlyAccess", "AliyunOSSReadOnlyAccess", "AliyunPubDNSReadOnlyAccess",]
}
