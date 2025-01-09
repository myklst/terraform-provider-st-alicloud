locals {
  users = ["lq-user-2", "lq-user-3", "lq-user-4", "lq-user-5", "lq-user-6", "lq-user-7", "lq-user-8", "lq-user-9", "lq-user-10", "lq-user-11"]
}

resource "st-alicloud_ram_policy" "ram_policy" {
  for_each = toset(local.users)

  attached_policies = ["AliyunRAMReadOnlyAccess", "AliyunOSSReadOnlyAccess", "AliyunECSReadOnlyAccess", "AliyunRDSReadOnlyAccess",
    "AliyunVPCReadOnlyAccess", "AliyunEIPReadOnlyAccess", "AliyunOCSReadOnlyAccess", "AliyunOTSReadOnlyAccess",
  "LqTestPolicy", "LqTestPolicy2"]
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
