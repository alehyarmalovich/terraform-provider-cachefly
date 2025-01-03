package cachefly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/exp/rand"
)

// Original makeRequest function remains unchanged
func makeRequest(client *CacheFlyClient, method, url string, body interface{}) (*http.Response, error) {
	var requestBody io.Reader

	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		requestBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

// makeRequestWithRetry makes an HTTP request with retry logic for transient errors.
func makeRequestWithRetry(client *CacheFlyClient, method, url string, body interface{}) (*http.Response, error) {
	const maxRetries = 5
	const baseDelay = time.Second // Start with 1 second delay

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := makeRequest(client, method, url, body)
		if err == nil && resp.StatusCode < 500 {
			// If no error and status code < 500, return the response
			return resp, nil
		}

		if resp != nil {
			// Read and log the response body for debugging purposes
			body, _ := io.ReadAll(resp.Body)
			log.Printf("[WARN] Request failed with status %d: %s. Retrying attempt %d/%d...",
				resp.StatusCode, string(body), attempt+1, maxRetries)
			resp.Body.Close()
		}

		// Log the error and retry
		if err != nil {
			log.Printf("[WARN] Request error: %v. Retrying attempt %d/%d...", err, attempt+1, maxRetries)
			lastErr = err
		}

		// Exponential backoff with jitter
		delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
		delay += time.Duration(rand.Intn(100)) * time.Millisecond // Add jitter
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries, lastErr)
}

// handleResponse validates HTTP response and returns an error if the status code is not 2xx.
func handleResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

// Helper: validateUniqueName
func validateUniqueName(val interface{}, key string) (warns []string, errs []error) {
	v := val.(string)
	if len(v) < 3 || len(v) > 32 || v != strings.ToLower(v) || !regexp.MustCompile(`^[a-z0-9]+$`).MatchString(v) {
		errs = append(errs, fmt.Errorf(
			"%q must be lowercase, alphanumeric, and between 3-32 characters long. Found: %s",
			key, v,
		))
	}
	return
}

// Simplified retrieval of data
func getString(data map[string]interface{}, key, defaultValue string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return defaultValue
}

func getBool(data map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := data[key].(bool); ok {
		return val
	}
	return defaultValue
}

func getInt(data map[string]interface{}, key string, defaultValue int) int {
	if val, ok := data[key].(int); ok {
		return val
	}
	return defaultValue
}

// Helper to manage additional configurations
func manageAdditionalConfigurations(client *CacheFlyClient, d *schema.ResourceData, serviceID string) error {
	if domains, ok := d.GetOk("domains"); ok {
		if err := manageServiceDomains(client, serviceID, domains.([]interface{})); err != nil {
			return err
		}
	}
	return nil
}

// Helper to deactivate a service
func deactivateService(client *CacheFlyClient, serviceID string) error {
	url := fmt.Sprintf("%s/api/2.5/services/%s/deactivate", client.APIURL, serviceID)
	resp, err := makeRequestWithRetry(client, "PUT", url, nil)
	if err != nil {
		return fmt.Errorf("failed to deactivate service after retries: %w", err)
	}
	defer resp.Body.Close()
	if err := handleResponse(resp); err != nil {
		return fmt.Errorf("failed to deactivate service: %w", err)
	}

	return nil
}

// Helper to manage service domains
func manageServiceDomains(client *CacheFlyClient, serviceID string, desiredDomains []interface{}) error {
	existingDomains, err := fetchExistingDomains(client, serviceID)
	if err != nil {
		return fmt.Errorf("failed to fetch existing domains: %v", err)
	}

	// Map existing domains by name for easy lookup
	existingDomainMap := mapExistingDomains(existingDomains)

	// Keep track of processed domains to avoid deleting active ones
	processedDomains := make(map[string]bool)

	// Add or update domains
	for _, domain := range desiredDomains {
		domainMap := domain.(map[string]interface{})
		name := domainMap["name"].(string)
		description := domainMap["description"].(string)
		validationMode := domainMap["validation_mode"].(string)

		if existingDomain, exists := existingDomainMap[name]; exists {
			// Update if the domain exists but differs in description or validation mode
			if needsUpdate(existingDomain, description, validationMode) {
				if err := updateServiceDomain(client, serviceID, existingDomain.ID, name, description, validationMode); err != nil {
					return fmt.Errorf("failed to update domain '%s': %v", name, err)
				}
			}
		} else {
			// Create a new domain if it doesn't exist
			if err := createServiceDomain(client, serviceID, name, description, validationMode); err != nil {
				return fmt.Errorf("failed to create domain '%s': %v", name, err)
			}
		}

		// Mark the domain as processed
		processedDomains[name] = true
	}

	// Delete domains that were not processed and are not default CacheFly domains
	return deleteUnusedDomains(client, serviceID, existingDomainMap, processedDomains)
}

// Helper function to fetch existing domains for a service
func fetchExistingDomains(client *CacheFlyClient, serviceID string) ([]DomainResource, error) {
	url := fmt.Sprintf("%s/api/2.5/services/%s/domains", client.APIURL, serviceID)
	resp, err := makeRequestWithRetry(client, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch domains after retries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch domains: HTTP %d, %s", resp.StatusCode, string(body))
	}

	var response struct {
		Domains []DomainResource `json:"data"` // Corrected JSON tag
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode domains response: %w", err)
	}

	return response.Domains, nil
}

// Helper to update a domain for a service
func updateServiceDomain(client *CacheFlyClient, serviceID, domainID, name, description, validationMode string) error {
	url := fmt.Sprintf("%s/api/2.5/services/%s/domains/%s", client.APIURL, serviceID, domainID)
	body := map[string]string{
		"name":           name,
		"description":    description,
		"validationMode": validationMode,
	}

	resp, err := makeRequestWithRetry(client, "PUT", url, body)
	if err != nil {
		return fmt.Errorf("failed to update service domain after retries: %w", err)
	}
	defer resp.Body.Close()

	if err := handleResponse(resp); err != nil {
		return fmt.Errorf("failed to update service domain: %w", err)
	}

	return nil
}

// Helper to create a domain for a service
func createServiceDomain(client *CacheFlyClient, serviceID, name, description, validationMode string) error {
	url := fmt.Sprintf("%s/api/2.5/services/%s/domains", client.APIURL, serviceID)
	body := map[string]string{
		"name":           name,
		"description":    description,
		"validationMode": validationMode,
	}

	resp, err := makeRequestWithRetry(client, "POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create service domain after retries: %w", err)
	}
	defer resp.Body.Close()

	if err := handleResponse(resp); err != nil {
		return fmt.Errorf("failed to create service domain: %w", err)
	}

	return nil
}

func deleteUnusedDomains(client *CacheFlyClient, serviceID string, existingDomainMap map[string]DomainResource, processedDomains map[string]bool) error {
	for name, existingDomain := range existingDomainMap {
		// Skip if the domain has been processed or is a default CacheFly domain
		if processedDomains[name] || isDefaultDomain(name) {
			continue
		}

		// Delete unused domains
		if err := deleteServiceDomain(client, serviceID, existingDomain.ID); err != nil {
			return fmt.Errorf("failed to delete domain '%s': %v", name, err)
		}
	}
	return nil
}

func deleteServiceDomain(client *CacheFlyClient, serviceID, domainID string) error {
	url := fmt.Sprintf("%s/api/2.5/services/%s/domains/%s", client.APIURL, serviceID, domainID)
	resp, err := makeRequestWithRetry(client, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to send DELETE request after retries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete domain: HTTP %d. Response: %s", resp.StatusCode, string(body))
	}

	return nil
}

func isDefaultDomain(name string) bool {
	return strings.HasSuffix(name, ".cachefly.net")
}

func needsUpdate(existing DomainResource, description, validationMode string) bool {
	return existing.Description != description || existing.ValidationMode != validationMode
}

func mapExistingDomains(domains []DomainResource) map[string]DomainResource {
	mapped := make(map[string]DomainResource)
	for _, domain := range domains {
		mapped[domain.Name] = domain
	}
	return mapped
}
