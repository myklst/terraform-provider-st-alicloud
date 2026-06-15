---
subcategory: "SLB (Server Load Balancer)"
layout: "st-alicloud"
page_title: "ST-Alicloud: st-alicloud_slb_listener_acl_attachment"
sidebar_current: "docs-st-alicloud-resource-slb-listener-acl-attachment"
description: |-
  Attach ACL(s) to an SLB listener and enable access control.
---

# st-alicloud_slb_listener_acl_attachment

Attach ACL(s) to an SLB listener and enable access control with white list type.

## Example Usage

```hcl
resource "st-alicloud_slb_listener_acl_attachment" "example" {
  listener_id = "lb-1234567890:tcp:80"
  acl_ids     = ["acl-1234567890", "acl-0987654321"]
}
```

## Argument Reference

The following arguments are supported:

* `listener_id` - (Required, ForceNew) The listener ID in the format `load_balancer_id:protocol:port` (e.g. `lb-xxx:tcp:80`).
* `acl_ids` - (Required) List of ACL IDs to attach to the listener.

## Attributes Reference

The following attributes are exported:

* `id` - Same as `listener_id`.

## Import

SLB Listener ACL Attachment can be imported using the `listener_id`, e.g.,

```shell
terraform import st-alicloud_slb_listener_acl_attachment.example lb-1234567890:tcp:80
```
