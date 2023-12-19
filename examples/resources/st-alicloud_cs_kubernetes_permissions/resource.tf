resource "st-alicloud_cs_kubernetes_permissions" "rbac" {
  uid = "201122334455667789"

  permissions {
    cluster     = "cxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
    role_type   = "cluster"
    role_name   = "test-permission"
    is_custom   = true
    is_ram_role = false
    namespace   = ""
  }
}
