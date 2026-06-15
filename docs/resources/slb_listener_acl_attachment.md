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
  load_balancer_id = "lb-1234567890"
  listener_port    = 80
  protocol         = "tcp"
  acl_ids          = ["acl-1234567890", "acl-0987654321"]
}
```

## Argument Reference

The following arguments are supported:

* `load_balancer_id` - (Required, ForceNew) The ID of the SLB instance.
* `listener_port` - (Required, ForceNew) The listener port.
* `protocol` - (Required, ForceNew) The listener protocol. Valid values: `http`, `https`, `tcp`, `udp`.
* `acl_ids` - (Required) List of ACL IDs to attach to the listener.

## Attributes Reference

The following attributes are exported:

* `id` - Resource ID, formatted as `load_balancer_id:protocol:listener_port`.
* `acl_status` - The access control status. Always `on` when ACL is attached.
* `acl_type` - The access control type. Always `white`.

## Import

SLB Listener ACL Attachment can be imported using the ID format `load_balancer_id:protocol:listener_port`, e.g.,

```shell
terraform import st-alicloud_slb_listener_acl_attachment.example lb-1234567890:tcp:80
```
