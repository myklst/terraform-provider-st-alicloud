provider "st-alicloud" {
  alias  = "slb"
  region = "cn-hongkong"
}

data "st-alicloud_slb_load_balancers" "slbs" {
  provider = st-alicloud.slb

  tags = {
    "app" = "web-server"
    "env" = "basic"
  }
}

output "slb_load_balancers" {
  value = data.st-alicloud_slb_load_balancers.slbs
}
