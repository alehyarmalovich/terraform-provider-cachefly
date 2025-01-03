package cachefly

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Account represents the structure of the CacheFly account API response.
type Account struct {
	ID          string `json:"_id"`
	CompanyName string `json:"companyName"`
	Website     string `json:"website"`
}

// dataSourceCacheflyAccount returns account information.
func dataSourceCacheflyAccount() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceCacheflyAccountRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The CacheFly account ID.",
			},
			"company_name": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The company name.",
			},
			"website": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The company's website.",
			},
		},
	}
}

func dataSourceCacheflyAccountRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*CacheFlyClient)

	resp, err := makeRequestWithRetry(client, "GET", fmt.Sprintf("%s/api/2.5/accounts/me", client.APIURL), nil)
	if err != nil {
		return diag.Errorf("failed to fetch account information after retries: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return diag.Errorf("failed to fetch account information: HTTP %d. Response: %s", resp.StatusCode, string(body))
	}

	var account Account
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return diag.Errorf("failed to decode account response: %v", err)
	}

	d.SetId(account.ID)
	d.Set("company_name", account.CompanyName)
	d.Set("website", account.Website)

	return nil
}
