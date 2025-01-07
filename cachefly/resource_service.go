package cachefly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

type ServiceResource struct {
	ID                string `json:"_id,omitempty"`
	UpdateAt          string `json:"updateAt,omitempty"`
	CreatedAt         string `json:"createdAt,omitempty"`
	Name              string `json:"name"`
	UniqueName        string `json:"uniqueName"`
	Description       string `json:"description,omitempty"`
	AutoSsl           bool   `json:"autoSsl,omitempty"`
	ConfigurationMode string `json:"configurationMode,omitempty"`
	Status            string `json:"status,omitempty"`
}

type DomainResource struct {
	ID               string   `json:"_id"`
	UpdateAt         string   `json:"updateAt"`
	CreatedAt        string   `json:"createdAt"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Service          string   `json:"service"`
	Certificates     []string `json:"certificates,omitempty"`
	ValidationMode   string   `json:"validationMode"`
	ValidationTarget string   `json:"validationTarget,omitempty"`
	ValidationStatus string   `json:"validationStatus,omitempty"`
}

type ReverseProxy struct {
	Enabled           bool   `json:"enabled"`
	Hostname          string `json:"hostname"`
	Mode              string `json:"mode"`
	OriginScheme      string `json:"originScheme"`
	CacheByQueryParam bool   `json:"cacheByQueryParam"`
	TTL               int    `json:"ttl"`
	UseRobotsTxt      bool   `json:"useRobotsTxt"`
	Prepend           string `json:"prepend,omitempty"`
	AccessKey         string `json:"accessKey"`
	SecretKey         string `json:"secretKey"`
	Region            string `json:"region"`
	Bucket            string `json:"bucket,omitempty"`
}

type ErrorTTL struct {
	Enabled bool `json:"enabled"`
	Value   *int `json:"value,omitempty"` // Pointer to distinguish between unset and zero
}

type ServiceOptions struct {
	ReverseProxy        ReverseProxy `json:"reverseProxy"`
	ErrorTTL            *ErrorTTL    `json:"error_ttl"`
	HostnamePassThrough bool         `json:"edgetoorigin"`
}

func resourceCacheflyService() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceCacheflyServiceCreate,
		ReadContext:   resourceCacheflyServiceRead,
		UpdateContext: resourceCacheflyServiceUpdate,
		DeleteContext: resourceCacheflyServiceDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceCacheflyServiceImport,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Service display name.",
			},
			"unique_name": {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "Service unique name, used to generate the default domain.",
				ValidateFunc: validateUniqueName,
				ForceNew:     true,
			},
			"description": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Description of the service.",
			},
			"auto_ssl": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Indicates whether AutoSSL is enabled for the service.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The status of the service (e.g., ACTIVE, Pending Configuration, DEACTIVATED).",
			},
			"domains": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The domain name (e.g., example.com).",
						},
						"description": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "A description of the domain.",
						},
						"validation_mode": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      "NONE",
							ValidateFunc: validation.StringInSlice([]string{"NONE", "MANUAL", "HTTP", "DNS"}, false),
							Description:  "The validation mode for the domain.",
						},
					},
				},
				Description: "A list of domains associated with the service.",
			},
			"cors": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable CORS headers for content.",
			},
			"auto_redirect": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable automatic redirect from HTTP to HTTPS.",
			},
			"reverse_proxy": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: reverseProxySchema(),
				},
			},
			"error_ttl": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enabled": {
							Type:        schema.TypeBool,
							Optional:    true,
							Computed:    true,
							Description: "Specifies whether error TTL is enabled.",
						},
						"value": {
							Type:        schema.TypeInt,
							Optional:    true,
							Computed:    true,
							Description: "The TTL value for errors in seconds.",
						},
					},
				},
			},
			"hostname_pass_through": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Enable or disable hostname pass-through (Edge to Origin).",
			},
		},
	}
}

func reverseProxySchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"hostname": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "The hostname for the reverse proxy. Required for all modes.",
		},
		"mode": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "WEB",
			Description:  "The mode of the reverse proxy. Must be either 'WEB' or 'OBJECT_STORAGE'.",
			ValidateFunc: validation.StringInSlice([]string{"WEB", "OBJECT_STORAGE"}, false),
		},
		"ttl": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      2592000,
			Description:  "Time-to-live for cached content in seconds. Range: 1 to 7776000. Required for all modes.",
			ValidateFunc: validation.IntBetween(1, 7776000),
		},
		"cache_by_query_param": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     false,
			Description: "Specifies whether to cache based on query parameters. Required for all modes.",
		},
		"origin_scheme": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "HTTPS",
			Description:  "Specifies the origin scheme. Allowed values are 'HTTP', 'HTTPS', or 'FOLLOW'. Required for all modes.",
			ValidateFunc: validation.StringInSlice([]string{"HTTP", "HTTPS", "FOLLOW"}, false),
		},
		"use_robots_txt": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     false,
			Description: "Specifies whether to respect the robots.txt file. Required for all modes.",
		},
		"access_key": {
			Type:        schema.TypeString,
			Optional:    true,
			Computed:    true,
			Description: "The access key for the OBJECT_STORAGE mode. Required for OBJECT_STORAGE.",
			Sensitive:   true,
		},
		"secret_key": {
			Type:        schema.TypeString,
			Optional:    true,
			Computed:    true,
			Description: "The secret key for the OBJECT_STORAGE mode. Required for OBJECT_STORAGE.",
			Sensitive:   true,
		},
		"region": {
			Type:        schema.TypeString,
			Optional:    true,
			Computed:    true,
			Description: "The region for the OBJECT_STORAGE mode. Required for OBJECT_STORAGE.",
		},
	}
}

// Resource Create
func resourceCacheflyServiceCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*CacheFlyClient)

	uniqueName := strings.ToLower(d.Get("unique_name").(string))
	serviceName := d.Get("name").(string)
	description := d.Get("description").(string)

	// Check for existing service
	existingService, err := findServiceByUniqueName(client, uniqueName)
	if err != nil {
		return diag.FromErr(err)
	}

	if existingService != nil {
		if existingService.Status == "DEACTIVATED" {
			// Reactivate the service if it is deactivated
			if err := reactivateService(client, existingService.ID); err != nil {
				return diag.Errorf("Failed to reactivate service: %v", err)
			}
			d.SetId(existingService.ID)
			return resourceCacheflyServiceRead(ctx, d, meta)
		}
		// Service exists but is active
		return diag.Errorf("Service with unique_name '%s' already exists.", uniqueName)
	}

	// Create a new service if none exists
	service := ServiceResource{
		Name:        serviceName,
		UniqueName:  uniqueName,
		Description: description,
	}
	resp, err := makeRequestWithRetry(client, "POST", fmt.Sprintf("%s/api/2.5/services", client.APIURL), service)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	if err := handleResponse(resp); err != nil {
		return diag.FromErr(err)
	}

	// Parse response
	var createdService ServiceResource
	if err := json.NewDecoder(resp.Body).Decode(&createdService); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(createdService.ID)

	// Configure reverse proxy if provided
	if v, ok := d.GetOk("reverse_proxy"); ok {
		reverseProxy := v.([]interface{})[0].(map[string]interface{})
		proxyConfig := ReverseProxy{
			Enabled:           true, // Automatically enabled if reverse_proxy block is provided
			Hostname:          reverseProxy["hostname"].(string),
			Mode:              reverseProxy["mode"].(string),
			OriginScheme:      reverseProxy["origin_scheme"].(string),
			CacheByQueryParam: reverseProxy["cache_by_query_param"].(bool),
			TTL:               reverseProxy["ttl"].(int),
			UseRobotsTxt:      reverseProxy["use_robots_txt"].(bool),
		}

		if reverseProxy["prepend"] != nil {
			proxyConfig.Prepend = reverseProxy["prepend"].(string)
		}

		if reverseProxy["access_key"] != nil {
			proxyConfig.AccessKey = reverseProxy["access_key"].(string)
		}
		if reverseProxy["secret_key"] != nil {
			proxyConfig.SecretKey = reverseProxy["secret_key"].(string)
		}
		if reverseProxy["region"] != nil {
			proxyConfig.Region = reverseProxy["region"].(string)
		}
		if reverseProxy["bucket"] != nil {
			proxyConfig.Bucket = reverseProxy["bucket"].(string)
		}

		// Configuring reverse proxy
		err := configureReverseProxy(client, createdService.ID, proxyConfig)
		if err != nil {
			return diag.Errorf("failed to configure reverse proxy: %v", err)
		}
	} else {
		// If reverse proxy is not provided, ensure it's disabled
		err := configureReverseProxy(client, createdService.ID, ReverseProxy{Enabled: false})
		if err != nil {
			return diag.Errorf("failed to disable reverse proxy: %v", err)
		}
	}

	// Configure error_ttl if provided
	if v, ok := d.GetOk("error_ttl"); ok {
		errorTTLConfig := v.([]interface{})[0].(map[string]interface{})
		errorTTL := map[string]interface{}{
			"enabled": errorTTLConfig["enabled"].(bool),
		}
		if value, ok := errorTTLConfig["value"]; ok {
			errorTTL["value"] = value.(int)
		}

		// Send the error_ttl configuration to the API
		payload := map[string]interface{}{
			"error_ttl": errorTTL,
		}
		url := fmt.Sprintf("%s/api/2.6/services/%s/options", client.APIURL, createdService.ID)
		resp, err := makeRequestWithRetry(client, "PUT", url, payload)
		if err != nil {
			return diag.Errorf("failed to configure error_ttl: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return diag.Errorf("API error while configuring error_ttl: %s", string(body))
		}
	}

	if err := manageAdditionalConfigurations(client, d, createdService.ID); err != nil {
		return diag.FromErr(err)
	}

	return resourceCacheflyServiceRead(ctx, d, meta)
}

func resourceCacheflyServiceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*CacheFlyClient)

	service, err := fetchServiceDetails(client, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if service == nil {
		d.SetId("")
		return nil
	}

	d.Set("name", service.Name)
	d.Set("unique_name", service.UniqueName)
	d.Set("description", service.Description)
	d.Set("auto_ssl", service.AutoSsl)
	d.Set("status", service.Status)

	reverseProxy, errorTTL, hostnamePassThrough, err := getServiceOptions(client, d.Id())
	if err != nil {
		return diag.Errorf("failed to fetch service options: %v", err)
	}

	if reverseProxy.Enabled {
		reverseProxyMap := map[string]interface{}{
			"hostname":             reverseProxy.Hostname,
			"mode":                 reverseProxy.Mode,
			"origin_scheme":        reverseProxy.OriginScheme,
			"ttl":                  reverseProxy.TTL,
			"use_robots_txt":       reverseProxy.UseRobotsTxt,
			"cache_by_query_param": reverseProxy.CacheByQueryParam,
			"access_key":           reverseProxy.AccessKey,
			"secret_key":           reverseProxy.SecretKey,
			"region":               reverseProxy.Region,
		}
		d.Set("reverse_proxy", []interface{}{reverseProxyMap})
	} else {
		d.Set("reverse_proxy", nil)
	}

	if _, ok := d.GetOk("error_ttl"); ok {
		if errorTTL != nil {
			errorTTLMap := map[string]interface{}{
				"enabled": errorTTL.Enabled,
			}
			if errorTTL.Value != nil {
				errorTTLMap["value"] = *errorTTL.Value
			}
			d.Set("error_ttl", []interface{}{errorTTLMap})
		} else {
			d.Set("error_ttl", nil)
		}
	} else {
		fmt.Println("Ignoring error_ttl as it is not defined in Terraform configuration.")
	}

	d.Set("hostname_pass_through", hostnamePassThrough)

	return nil
}

func fetchServiceDetails(client *CacheFlyClient, serviceID string) (*ServiceResource, error) {
	url := fmt.Sprintf("%s/api/2.5/services/%s", client.APIURL, serviceID)
	resp, err := makeRequestWithRetry(client, "GET", url, nil) // Use retry wrapper
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read service: %s", resp.Status)
	}

	var service ServiceResource
	if err := json.NewDecoder(resp.Body).Decode(&service); err != nil {
		return nil, err
	}

	return &service, nil
}

func resourceCacheflyServiceUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*CacheFlyClient)
	serviceID := d.Id()
	var diags diag.Diagnostics

	// Update description if it has changed
	if d.HasChange("description") {
		newDescription := d.Get("description").(string)
		service := ServiceResource{
			Description: newDescription,
		}
		err := updateServiceDetails(client, serviceID, &service)
		if err != nil {
			diags = append(diags, diag.Errorf("failed to update description: %v", err)...)
		}
	}

	// Fetch current service options
	reverseProxy, _, _, err := getServiceOptions(client, serviceID)
	if err != nil {
		return diag.Errorf("failed to fetch service options: %v", err)
	}

	// Handle reverse proxy updates
	if d.HasChange("reverse_proxy") {
		if v, ok := d.GetOk("reverse_proxy"); ok {
			reverseProxyConfig := v.([]interface{})[0].(map[string]interface{})
			proxyConfig := ReverseProxy{
				Enabled:           true, // Automatically enabled when the block is present
				Hostname:          reverseProxyConfig["hostname"].(string),
				Mode:              reverseProxyConfig["mode"].(string),
				OriginScheme:      reverseProxyConfig["origin_scheme"].(string),
				CacheByQueryParam: reverseProxyConfig["cache_by_query_param"].(bool),
				TTL:               reverseProxyConfig["ttl"].(int),
				UseRobotsTxt:      reverseProxyConfig["use_robots_txt"].(bool),
			}

			if reverseProxyConfig["prepend"] != nil {
				proxyConfig.Prepend = reverseProxyConfig["prepend"].(string)
			}
			if reverseProxyConfig["access_key"] != nil {
				proxyConfig.AccessKey = reverseProxyConfig["access_key"].(string)
			}
			if reverseProxyConfig["secret_key"] != nil {
				proxyConfig.SecretKey = reverseProxyConfig["secret_key"].(string)
			}
			if reverseProxyConfig["region"] != nil {
				proxyConfig.Region = reverseProxyConfig["region"].(string)
			}
			if reverseProxyConfig["bucket"] != nil {
				proxyConfig.Bucket = reverseProxyConfig["bucket"].(string)
			}

			// Configure reverse proxy
			err = configureReverseProxy(client, serviceID, proxyConfig)
			if err != nil {
				return diag.Errorf("failed to update reverse proxy: %v", err)
			}
		} else {
			// If reverse proxy block is removed, disable it
			if reverseProxy.Enabled {
				err := configureReverseProxy(client, serviceID, ReverseProxy{Enabled: false})
				if err != nil {
					return diag.Errorf("failed to disable reverse proxy: %v", err)
				}
			}
		}
	}

	// Handle error_ttl updates
	if d.HasChange("error_ttl") {
		if v, ok := d.GetOk("error_ttl"); ok {
			errorTTLConfig := v.([]interface{})[0].(map[string]interface{})
			errorTTL := map[string]interface{}{
				"enabled": errorTTLConfig["enabled"].(bool),
			}
			if value, ok := errorTTLConfig["value"]; ok {
				errorTTL["value"] = value.(int)
			}

			payload := map[string]interface{}{
				"error_ttl": errorTTL,
			}
			url := fmt.Sprintf("%s/api/2.6/services/%s/options", client.APIURL, serviceID)
			resp, err := makeRequestWithRetry(client, "PUT", url, payload)
			if err != nil {
				return diag.Errorf("failed to update error_ttl: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return diag.Errorf("API error while updating error_ttl: %s", string(body))
			}
		} else {
			payload := map[string]interface{}{
				"error_ttl": map[string]interface{}{
					"enabled": false,
				},
			}
			url := fmt.Sprintf("%s/api/2.6/services/%s/options", client.APIURL, serviceID)
			resp, err := makeRequestWithRetry(client, "PUT", url, payload)
			if err != nil {
				return diag.Errorf("failed to disable error_ttl: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return diag.Errorf("API error while disabling error_ttl: %s", string(body))
			}
		}
	}

	if d.HasChange("hostname_pass_through") {
		hostnamePassThrough := d.Get("hostname_pass_through").(bool)

		payload := map[string]interface{}{
			"edgetoorigin": hostnamePassThrough,
		}

		url := fmt.Sprintf("%s/api/2.6/services/%s/options", client.APIURL, serviceID)
		resp, err := makeRequestWithRetry(client, "PUT", url, payload)
		if err != nil {
			return diag.Errorf("failed to update hostname_pass_through: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return diag.Errorf("API error while updating hostname_pass_through: %s", string(body))
		}
	}

	// Manage domains if they have changed
	if d.HasChange("domains") {
		newDomains := d.Get("domains").([]interface{})
		err := manageServiceDomains(client, serviceID, newDomains)
		if err != nil {
			diags = append(diags, diag.Errorf("failed to update domains: %v", err)...)
		}
	}

	diags = append(diags, resourceCacheflyServiceRead(ctx, d, meta)...)

	return diags
}

// updateServiceDetails updates specific details of a CacheFly service.
func updateServiceDetails(client *CacheFlyClient, serviceID string, service *ServiceResource) error {
	url := fmt.Sprintf("%s/api/2.5/services/%s", client.APIURL, serviceID)

	// Convert the service object to JSON payload
	payload, err := json.Marshal(service)
	if err != nil {
		return fmt.Errorf("failed to marshal service payload: %w", err)
	}

	// Create a PUT request to update the service details
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+client.Token)
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Handle response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s. Response: %s", resp.Status, string(body))
	}

	return nil
}

// Resource Delete
func resourceCacheflyServiceDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*CacheFlyClient)

	if err := deactivateService(client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func findServiceByUniqueName(client *CacheFlyClient, uniqueName string) (*ServiceResource, error) {
	url := fmt.Sprintf("%s/api/2.5/services?responseType=full", client.APIURL)
	resp, err := makeRequestWithRetry(client, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch services after retries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch services: HTTP %d. Response: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Services []ServiceResource `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode services response: %w", err)
	}

	for _, service := range response.Services {
		if service.UniqueName == uniqueName {
			return &service, nil
		}
	}

	return nil, nil
}

func reactivateService(client *CacheFlyClient, serviceID string) error {
	url := fmt.Sprintf("%s/api/2.5/services/%s/activate", client.APIURL, serviceID)

	resp, err := makeRequestWithRetry(client, "PUT", url, nil)
	if err != nil {
		return fmt.Errorf("failed to send activation request after retries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to activate service: HTTP %d. Response: %s", resp.StatusCode, string(body))
	}

	return nil
}

func configureReverseProxy(client *CacheFlyClient, serviceID string, reverseProxy ReverseProxy) error {
	url := fmt.Sprintf("%s/api/2.6/services/%s/options", client.APIURL, serviceID)

	// Fetching the current state of the reverse proxy
	currentState, _, _, err := getServiceOptions(client, serviceID)
	if err != nil {
		return fmt.Errorf("failed to fetch current reverse proxy state: %w", err)
	}

	// Automatically enable reverse proxy if the configuration is provided
	if reverseProxy.Hostname != "" {
		reverseProxy.Enabled = true
	}

	if !currentState.Enabled && !reverseProxy.Enabled {
		fmt.Println("Reverse proxy is already disabled. No action required.")
		return nil
	}

	if !reverseProxy.Enabled {
		payload := map[string]interface{}{
			"reverseProxy": map[string]interface{}{
				"enabled": false,
			},
		}

		fmt.Printf("Disabling reverse proxy: %s\n", url)
		resp, err := makeRequestWithRetry(client, "PUT", url, payload)
		if err != nil {
			return fmt.Errorf("failed to disable reverse proxy: %w", err)
		}
		defer resp.Body.Close()

		// Проверяем успешность ответа
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Response Status: %d\n", resp.StatusCode)
		fmt.Printf("Response Body: %s\n", string(body))

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
		}
		return nil
	}

	payload := map[string]interface{}{
		"reverseProxy": map[string]interface{}{
			"enabled":           true,
			"hostname":          reverseProxy.Hostname,
			"mode":              reverseProxy.Mode,
			"ttl":               reverseProxy.TTL,
			"cacheByQueryParam": reverseProxy.CacheByQueryParam,
			"originScheme":      reverseProxy.OriginScheme,
			"useRobotsTxt":      reverseProxy.UseRobotsTxt,
		},
	}

	if reverseProxy.Mode == "WEB" && reverseProxy.Prepend != "" {
		payload["reverseProxy"].(map[string]interface{})["prepend"] = reverseProxy.Prepend
	}

	if reverseProxy.Mode == "OBJECT_STORAGE" {
		if reverseProxy.AccessKey == "" || reverseProxy.SecretKey == "" || reverseProxy.Region == "" {
			return fmt.Errorf("accessKey, secretKey, and region are required for OBJECT_STORAGE mode")
		}
		payload["reverseProxy"].(map[string]interface{})["accessKey"] = reverseProxy.AccessKey
		payload["reverseProxy"].(map[string]interface{})["secretKey"] = reverseProxy.SecretKey
		payload["reverseProxy"].(map[string]interface{})["region"] = reverseProxy.Region

		if reverseProxy.Bucket != "" {
			payload["reverseProxy"].(map[string]interface{})["bucket"] = reverseProxy.Bucket
		}
	}

	fmt.Printf("Configuring reverse proxy: %s\n", url)
	resp, err := makeRequestWithRetry(client, "PUT", url, payload)
	if err != nil {
		return fmt.Errorf("failed to configure reverse proxy: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Response Status: %d\n", resp.StatusCode)
	fmt.Printf("Response Body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func getServiceOptions(client *CacheFlyClient, serviceID string) (ReverseProxy, *ErrorTTL, bool, error) {
	url := fmt.Sprintf("%s/api/2.6/services/%s/options", client.APIURL, serviceID)

	resp, err := makeRequestWithRetry(client, "GET", url, nil)
	if err != nil {
		return ReverseProxy{}, nil, false, fmt.Errorf("failed to fetch service options: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return ReverseProxy{}, nil, false, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var options ServiceOptions
	if err := json.NewDecoder(resp.Body).Decode(&options); err != nil {
		return ReverseProxy{}, nil, false, fmt.Errorf("failed to decode response: %w", err)
	}

	if options.ErrorTTL != nil && options.ErrorTTL.Value == nil {
		options.ErrorTTL = nil
	}

	return options.ReverseProxy, options.ErrorTTL, options.HostnamePassThrough, nil
}

func resourceCacheflyServiceImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	client := meta.(*CacheFlyClient)

	serviceID := d.Id()

	service, err := fetchServiceDetails(client, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service details for import: %w", err)
	}

	if service == nil {
		return nil, fmt.Errorf("service with ID %s not found", serviceID)
	}

	d.Set("name", service.Name)
	d.Set("unique_name", service.UniqueName)
	d.Set("description", service.Description)
	d.Set("auto_ssl", service.AutoSsl)
	d.Set("status", service.Status)

	return []*schema.ResourceData{d}, nil
}
