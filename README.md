# Terraform Provider for CacheFly

The `terraform-provider-cachefly` enables seamless integration with CacheFly's CDN platform, allowing you to manage and configure CacheFly resources using Terraform.

## Installation

To use the CacheFly Terraform provider, ensure you have Terraform installed and configured.

### Step 1: Add the Provider to Your Terraform Configuration

```hcl
terraform {
  required_providers {
    cachefly = {
      source = "alehyarmalovich/cachefly"
    }
  }
}

provider "cachefly" {
  token = var.cachefly_api_token
}
```
