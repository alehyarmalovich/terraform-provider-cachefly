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

// Service represents a single service object returned by the API.
type Service struct {
	ID                string `json:"_id"`
	Name              string `json:"name"`
	UniqueName        string `json:"uniqueName"`
	AutoSsl           bool   `json:"autoSsl"`
	ConfigurationMode string `json:"configurationMode"`
	Status            string `json:"status"`
}

// ServicesResponse represents the structure of the API response for services.
type ServicesResponse struct {
	Meta struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Count  int `json:"count"`
	} `json:"meta"`
	Data []Service `json:"data"`
}

// dataSourceCacheflyServices defines the schema for the data source.
func dataSourceCacheflyServices() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceCacheflyServicesRead,
		Schema: map[string]*schema.Schema{
			"response_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "shallow",
				Description: "The response type for the query. Possible values: ids, shallow, selected, full.",
			},
			"status": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Filter services by status (e.g., ACTIVE, DEACTIVATED).",
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
			"services": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "A list of services with their details.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The ID of the service.",
						},
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The name of the service.",
						},
						"unique_name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The unique name of the service.",
						},
						"auto_ssl": {
							Type:        schema.TypeBool,
							Computed:    true,
							Description: "Whether AutoSSL is enabled for the service.",
						},
						"configuration_mode": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The configuration mode of the service.",
						},
						"status": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The status of the service (e.g., ACTIVE, Pending Configuration).",
						},
					},
				},
			},
		},
	}
}

func dataSourceCacheflyServicesRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*CacheFlyClient)
	queryParams := url.Values{}
	queryParams.Set("responseType", d.Get("response_type").(string))
	if status, ok := d.GetOk("status"); ok {
		queryParams.Set("status", status.(string))
	}
	queryParams.Set("limit", strconv.Itoa(d.Get("limit").(int)))
	queryParams.Set("offset", strconv.Itoa(d.Get("offset").(int)))

	requestURL := fmt.Sprintf("%s/api/2.5/services?%s", client.APIURL, queryParams.Encode())

	resp, err := makeRequestWithRetry(client, "GET", requestURL, nil)
	if err != nil {
		return diag.Errorf("failed to fetch services after retries: %v", err)
	}
	defer resp.Body.Close()

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return diag.Errorf("API returned non-200 status: %d. Response: %s", resp.StatusCode, string(body))
	}

	var response ServicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return diag.Errorf("failed to decode services response: %v", err)
	}

	services := make([]map[string]interface{}, len(response.Data))
	for i, service := range response.Data {
		services[i] = map[string]interface{}{
			"id":                 service.ID,
			"name":               service.Name,
			"unique_name":        service.UniqueName,
			"auto_ssl":           service.AutoSsl,
			"configuration_mode": service.ConfigurationMode,
			"status":             service.Status,
		}
	}

	if err := d.Set("services", services); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("cachefly_services")
	return nil
}
