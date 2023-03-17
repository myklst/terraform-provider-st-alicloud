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

resource "st-alicloud_alicloud_ram_group_membership" "test" {
  group_name = "devops-group-02"
  user_name = "devopsuser02"
}

resource "st-alicloud_alicloud_ram_group_membership" "test2" {
  group_name = "devops-group-02"
  user_name = "devopsuser01"
}
