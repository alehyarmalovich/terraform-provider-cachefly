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

type Origin struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
}

type OriginsResponse struct {
	Meta struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Count  int `json:"count"`
	} `json:"meta"`
	Data []Origin `json:"data"`
}

func dataSourceCacheflyOrigins() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceCacheflyOriginsRead,
		Schema: map[string]*schema.Schema{
			"type": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The type of origins to list.",
			},
			"offset": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     0,
				Description: "Number of results to skip.",
			},
			"limit": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     10,
				Description: "Maximum number of results to return.",
			},
			"response_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "shallow",
				Description: "The response type for the query. Possible values: ids, shallow, selected, full.",
			},
			"origins": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "A list of origins.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The ID of the origin.",
						},
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The name of the origin.",
						},
					},
				},
			},
		},
	}
}

func dataSourceCacheflyOriginsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*CacheFlyClient)

	queryParams := url.Values{}
	if t, ok := d.GetOk("type"); ok {
		queryParams.Set("type", t.(string))
	}
	queryParams.Set("offset", strconv.Itoa(d.Get("offset").(int)))
	queryParams.Set("limit", strconv.Itoa(d.Get("limit").(int)))
	queryParams.Set("responseType", d.Get("response_type").(string))

	requestURL := fmt.Sprintf("%s/api/2.5/origins?%s", client.APIURL, queryParams.Encode())

	resp, err := makeRequestWithRetry(client, "GET", requestURL, nil)
	if err != nil {
		return diag.Errorf("failed to fetch origins after retries: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return diag.Errorf("API returned non-200 status: %d. Response: %s", resp.StatusCode, string(body))
	}

	var response OriginsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return diag.Errorf("failed to decode origins response: %v", err)
	}

	origins := make([]map[string]interface{}, len(response.Data))
	for i, origin := range response.Data {
		origins[i] = map[string]interface{}{
			"id":   origin.ID,
			"name": origin.Name,
		}
	}

	if err := d.Set("origins", origins); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("cachefly_origins")
	return nil
}
