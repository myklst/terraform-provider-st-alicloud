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
