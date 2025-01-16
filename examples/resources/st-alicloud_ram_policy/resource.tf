locals {
  users = ["lq-user-2", "lq-user-3"] //custom users used for testing. Please either create said users or remove them from this list.
}

resource "st-alicloud_ram_policy" "ram_policy" {
  for_each = toset(local.users)

  attached_policies = [
    "AliyunRAMReadOnlyAccess", "AliyunOSSReadOnlyAccess",
  "LqTestPolicy", "AliyunPubDNSReadOnlyAccess"] //custom policies used for testing. Please either create said policies or remove them from this list.
  user_name = each.value
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
