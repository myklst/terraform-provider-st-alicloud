terraform {
  required_providers {
    st-alicloud = {
      source = "myklst/st-alicloud"
    }
  }
}

provider "st-alicloud" {
  region = "cn-hongkong"
}

data "st-alicloud_cdn_domain" "def" {
  domain_name = "test.example.com"
}
