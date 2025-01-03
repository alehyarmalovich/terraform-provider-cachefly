package cachefly

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Domain represents a single domain object returned by the API.
type Domain struct {
	ID             string `json:"_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	ValidationMode string `json:"validationMode"`
	ValidationStatus string `json:"validationStatus"`
}

// DomainsResponse represents the structure of the API response for domains.
type DomainsResponse struct {
	Meta struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Count  int `json:"count"`
	} `json:"meta"`
	Data []Domain `json:"data"`
}

// Data Source: List Service Domains
func dataSourceCacheflyServiceDomains() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceCacheflyServiceDomainsRead,
		Schema: map[string]*schema.Schema{
			"service_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The ID of the service to fetch domains for.",
			},
			"search": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Search term to filter domains by name.",
			},
			"limit": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     10,
				Description: "The maximum number of results to return.",
			},
			"offset": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     0,
				Description: "The offset for the query (number of results to skip).",
			},
			"domains": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "A list of domains associated with the service.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The ID of the domain.",
						},
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The name of the domain.",
						},
						"description": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "A description of the domain.",
						},
						"validation_mode": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The validation mode for the domain (e.g., NONE, MANUAL, HTTP, DNS).",
						},
						"validation_status": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The validation status of the domain.",
						},
					},
				},
			},
		},
	}
}

func dataSourceCacheflyServiceDomainsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*CacheFlyClient)

	// Retrieve parameters
	serviceID := d.Get("service_id").(string)
	queryParams := url.Values{}

	if search, ok := d.GetOk("search"); ok {
		queryParams.Set("search", search.(string))
	}
	queryParams.Set("offset", strconv.Itoa(d.Get("offset").(int)))
	queryParams.Set("limit", strconv.Itoa(d.Get("limit").(int)))
	queryParams.Set("responseType", "shallow") // Set responseType default value if required

	requestURL := fmt.Sprintf("%s/api/2.5/services/%s/domains?%s", client.APIURL, serviceID, queryParams.Encode())

	// Make API request
	resp, err := makeRequestWithRetry(client, "GET", requestURL, nil)
	if err != nil {
		return diag.Errorf("failed to fetch service domains after retries: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return diag.Errorf("API returned non-200 status: %d. Response: %s", resp.StatusCode, string(body))
	}

	// Parse API response
	var response DomainsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return diag.Errorf("failed to decode service domains response: %v", err)
	}

	// Map response data to Terraform schema
	domains := make([]map[string]interface{}, len(response.Data))
	for i, domain := range response.Data {
		domains[i] = map[string]interface{}{
			"id":                domain.ID,
			"name":              domain.Name,
			"description":       domain.Description,
			"validation_mode":   domain.ValidationMode,
			"validation_status": domain.ValidationStatus,
		}
	}

	if err := d.Set("domains", domains); err != nil {
		return diag.FromErr(err)
	}

	// Set the resource ID
	d.SetId(serviceID)
	return nil
}
