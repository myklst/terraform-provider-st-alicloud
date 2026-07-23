---
subcategory: "Redis (R-Kvstore)"
layout: "alicloud"
page_title: "ST-Alicloud: kvstore_additional_bandwidth"
description: |-
  Manages additional bandwidth and elastic burst for an Alibaba Cloud Redis instance.
---

# st-alicloud_kvstore_additional_bandwidth

Manages additional bandwidth and elastic burst for an Alibaba Cloud Redis (R-Kvstore) instance.

This is a **unified resource** that handles two use cases:

1. **Instance-level burst** (elastic bandwidth): Omit `node_id` (or set to `"All"`) to enable/disable burst for the entire instance. Burst allows temporary bandwidth spikes beyond the instance's base limit.

2. **Per-shard additional bandwidth**: Set `node_id` to a specific shard's InsName (e.g. `r-xxxxx-db-0`) to purchase additional permanent bandwidth for that shard.

## Example Usage

### Instance-level burst only

```hcl
resource "st-alicloud_kvstore_additional_bandwidth" "burst" {
  instance_id     = "r-xxxxx"
  bandwidth_burst = true
}
```

### Per-shard additional bandwidth

```hcl
resource "st-alicloud_kvstore_additional_bandwidth" "shard_0" {
  instance_id = "r-xxxxx"
  node_id     = "r-xxxxx-db-0"
  bandwidth   = 20
}
```

### Both burst and per-shard bandwidth

```hcl
resource "st-alicloud_kvstore_additional_bandwidth" "burst" {
  instance_id     = "r-xxxxx"
  bandwidth_burst = true
}

resource "st-alicloud_kvstore_additional_bandwidth" "shard_0" {
  instance_id = "r-xxxxx"
  node_id     = "r-xxxxx-db-0"
  bandwidth   = 20
}
```

## Argument Reference

The following arguments are supported:

* `instance_id` - (Required, Forces new resource) The ID of the Redis instance.
* `node_id` - (Optional, Forces new resource) The shard (node) ID for per-shard bandwidth, in InsName format (e.g. `r-xxxxx-db-0`). Omit or set to `"All"` for instance-level burst. Use `DescribeRoleZoneInfo` or `DescribeLogicInstanceTopology` to list available node IDs.
* `bandwidth` - (Optional) Additional bandwidth in MB/s. Set to `0` (default) for instance-level burst only. Must be a positive integer for per-shard additional bandwidth. The API rejects values exceeding the instance's bandwidth limit.
* `bandwidth_burst` - (Optional) Whether to enable bandwidth burst. Defaults to `true`.

## Attribute Reference

The following attributes are exported:

* `id` - The resource ID. Format: `instance_id` (instance-level) or `instance_id:node_id` (per-shard).

## Import

Redis additional bandwidth can be imported using the following formats:

**Instance-level burst:**
```shell
terraform import st-alicloud_kvstore_additional_bandwidth.burst r-xxxxx
```

**Per-shard additional bandwidth:**
```shell
terraform import st-alicloud_kvstore_additional_bandwidth.shard_0 r-xxxxx:r-xxxxx-db-0
```

## Notes

* Burst is an instance-level setting — when enabled, the instance can temporarily exceed its bandwidth limit.
* Per-shard bandwidth is permanent additional bandwidth purchased for a specific shard.
* Both operations use the `EnableAdditionalBandwidth` API with different `NodeId` values (`All` vs specific shard).
* Deleting the resource resets bandwidth to default (0 additional, burst disabled).
* The API may take 2-4 minutes to complete as the instance goes through `Changing` → `Normal` status.
* If applying both burst and per-shard resources simultaneously, the provider retries on concurrent operation errors. Use `depends_on` for cleaner sequential ordering.
