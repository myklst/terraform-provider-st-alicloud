resource "st-alicloud_foasconsole_namespace" "namespace" {
  instance_id = "f_intl-sg-xxxxxxxxxxx"
  namespace   = "test-default"
  region      = "cn-hongkong"
  ha          = false

  guaranteed_resource_spec = {
    cpu       = 0
    memory_gb = 0
  }

  elastic_resource_spec = {
    cpu       = 10
    memory_gb = 40
  }
}
