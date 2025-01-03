package cachefly

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider initializes and returns the CacheFly Terraform provider.
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"api_url": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("CACHEFLY_API_URL", "https://api.cachefly.com"),
				Description: "The base URL for the CacheFly API.",
			},
			"token": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("CACHEFLY_TOKEN", nil),
				Description: "The API token for authenticating with the CacheFly API. Can also be set using the CACHEFLY_TOKEN environment variable.",
				Sensitive:   true,
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"cachefly_service": resourceCacheflyService(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"cachefly_account":         dataSourceCacheflyAccount(),
			"cachefly_services":        dataSourceCacheflyServices(),
			"cachefly_origins":         dataSourceCacheflyOrigins(),
			"cachefly_service_domains": dataSourceCacheflyServiceDomains(),
		},
		ConfigureContextFunc: providerConfigure,
	}
}

// providerConfigure configures the provider and returns a CacheFly client.
func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Get API URL
	apiURL := d.Get("api_url").(string)
	if apiURL == "" {
		apiURL = "https://api.cachefly.com"
	}

	// Get API Token from provider configuration or environment variable
	token, ok := d.GetOk("token")
	if !ok || token.(string) == "" {
		token = os.Getenv("CACHEFLY_TOKEN")
	}

	// Validate the token
	if token == "" || token.(string) == "" {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Invalid API Token",
			Detail:   "The 'token' must be provided either in the provider configuration or through the CACHEFLY_TOKEN environment variable.",
		})
		return nil, diags
	}

	// Initialize CacheFly client
	client := NewCacheFlyClient(apiURL, token.(string))
	if client == nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Failed to initialize CacheFly client",
			Detail:   fmt.Sprintf("Failed to create CacheFly client with the provided API URL: %s", apiURL),
		})
		return nil, diags
	}

	return client, diags
}
