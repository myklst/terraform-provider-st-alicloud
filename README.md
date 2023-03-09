Terraform Custom Provider for Alibaba Cloud
===========================================

This Terraform custom provider is designed for own use case scenario.

References
----------

- Website: https://www.terraform.io
- AliCloud official Terraform provider: https://github.com/aliyun/terraform-provider-alicloud

Supported Versions
------------------

| Terraform version | minimum provider version |maxmimum provider version
| ---- | ---- | ----|
| >= 1.3.x	| 1.0.0	| latest |

Requirements
------------

-	[Terraform](https://www.terraform.io/downloads.html) 1.3.x
-	[Go](https://golang.org/doc/install) 1.19 (to build the provider plugin)

Local Installation
------------------

1. Run make file `make install-local-custom-provider` to install the provider under ~/.terraform.d/plugins.

2. The provider source should be change to the path that configured in the *Makefile*:

    ```
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
    ```
