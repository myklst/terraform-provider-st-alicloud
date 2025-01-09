resource "st-alicloud_ram_policy" "ram_policy" {
  attached_policies = ["AliyunRAMReadOnlyAccess", "LqTestPolicy", "LqTestPolicy2"]
  user_name         = "lq-user-2"
}

terraform {
  required_providers {
    st-alicloud = {
      source = "example.local/myklst/st-alicloud"
    }
  }
}

provider "st-alicloud" {
  region = "cn-hongkong"
}
