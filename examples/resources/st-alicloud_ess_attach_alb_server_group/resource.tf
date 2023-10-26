resource "st-alicloud_ess_attach_alb_server_group" "attach_alb" {
  scaling_group_id = "asg-xxxxxxxxxxxxxxxxxxxx"
  alb_server_groups {
    alb_server_group_id = "sgp-xxxxxxxxxxxxxxxxxx"
    weight              = 100
    port                = 443
  }
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
