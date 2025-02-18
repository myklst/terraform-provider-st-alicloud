---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "st-alicloud_cs_user_kubeconfig Data Source - st-alicloud"
subcategory: ""
description: |-
  This data source provides the Kubeconfig of container service for the set Alibaba Cloud user.
---

# st-alicloud_cs_user_kubeconfig (Data Source)

This data source provides the Kubeconfig of container service for the set Alibaba Cloud user.

## Example Usage

```terraform
data "st-alicloud_cs_user_kubeconfig" "def" {
  cluster_id = "c-123"

  client_config {
    region     = "cn-hongkong"
    access_key = "<access-key>"
    secret_key = "<secret-key>"
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `cluster_id` (String) Cluster ID of container service for Kubernetes.

### Optional

- `client_config` (Block, Optional) Config to override default client created in Provider. This block will not be recorded in state file. (see [below for nested schema](#nestedblock--client_config))

### Read-Only

- `kubeconfig` (String, Sensitive) Kubeconfig of container service for Kubernetes.

<a id="nestedblock--client_config"></a>
### Nested Schema for `client_config`

Optional:

- `access_key` (String) The access key for user to query Kubeconfig. Default to use access key configured in the provider.
- `region` (String) The region of the Container Service for Kubernetes. Default to use region configured in the provider.
- `secret_key` (String) The secret key for user to query Kubeconfig. Default to use secret key configured in the provider.
