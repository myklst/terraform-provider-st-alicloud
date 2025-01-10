locals {
  users = ["lq-user-2"]
}

resource "st-alicloud_ram_policy" "ram_policy" {
  for_each = toset(local.users)

  attached_policies = [
    "AliyunRAMReadOnlyAccess", "AliyunOSSReadOnlyAccess", "AliyunECSReadOnlyAccess", "AliyunRDSReadOnlyAccess",
    "AliyunVPCReadOnlyAccess", "AliyunEIPReadOnlyAccess", "AliyunOCSReadOnlyAccess", "AliyunOTSReadOnlyAccess",
    "AliyunRDSReadOnlyAccess", "AliyunSLBReadOnlyAccess", "AliyunCDNReadOnlyAccess", "AliyunLogReadOnlyAccess",
    "AliyunCDNReadOnlyAccess",
  "LqTestPolicy", "LqTestPolicy2", ] //custom policies used for testing. Please either create said policies or remove them from this list.
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
