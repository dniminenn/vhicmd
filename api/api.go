package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/google/uuid"
	"github.com/jessegalley/vhicmd/internal/httpclient"
)

// ApiResponse represents the response from an API call
type ApiResponse struct {
	TokenHeader  string
	ResponseCode int
	Response     string
}

// callPOST is a helper for POST requests. If you need to pass a token, supply it via the `token` parameter.
func callPOST(url, token string, body interface{}) (ApiResponse, error) {
	apiResp := ApiResponse{}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return apiResp, fmt.Errorf("error marshaling JSON payload: %v", err)
	}

	resp, err := httpclient.SendRequestWithToken("POST", url, token, bytes.NewBuffer(jsonData))
	if err != nil {
		return apiResp, fmt.Errorf("error making HTTP POST request: %v", err)
	}
	defer resp.Body.Close()

	apiResp.ResponseCode = resp.StatusCode

	if token := resp.Header.Get("X-Subject-Token"); token != "" {
		apiResp.TokenHeader = token
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiResp, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode >= 400 {
		apiResp.Response = FormatErrorResponse(string(bodyBytes))
	} else {
		apiResp.Response = string(bodyBytes)
	}

	return apiResp, nil
}

// callGET is a helper for GET requests that requires a token in the X-Auth-Token header.
func callGET(url, token string) (ApiResponse, error) {
	apiResp := ApiResponse{}

	resp, err := httpclient.SendRequestWithToken("GET", url, token, nil)
	if err != nil {
		return apiResp, fmt.Errorf("error making HTTP GET request: %v", err)
	}
	defer resp.Body.Close()

	apiResp.ResponseCode = resp.StatusCode

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiResp, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode >= 400 {
		// Format error responses
		apiResp.Response = FormatErrorResponse(string(body))
	} else {
		apiResp.Response = string(body)
	}

	return apiResp, nil
}

// callDELETE is a helper for DELETE requests
func callDELETE(url, token string) (ApiResponse, error) {
	apiResp := ApiResponse{}

	resp, err := httpclient.SendRequestWithToken("DELETE", url, token, nil)
	if err != nil {
		return apiResp, fmt.Errorf("error making HTTP DELETE request: %v", err)
	}
	defer resp.Body.Close()

	apiResp.ResponseCode = resp.StatusCode

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiResp, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode >= 400 {
		apiResp.Response = FormatErrorResponse(string(body))
	} else {
		apiResp.Response = string(body)
	}

	return apiResp, nil
}

// callPATCH helper for PATCH requests
func callPATCH(url, token string, body interface{}) (ApiResponse, error) {
	apiResp := ApiResponse{}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return apiResp, fmt.Errorf("error marshaling JSON payload: %v", err)
	}

	resp, err := httpclient.SendRequestWithToken("PATCH", url, token, bytes.NewBuffer(jsonData))
	if err != nil {
		return apiResp, fmt.Errorf("error making HTTP PATCH request: %v", err)
	}
	defer resp.Body.Close()

	apiResp.ResponseCode = resp.StatusCode

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiResp, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode >= 400 {
		apiResp.Response = FormatErrorResponse(string(bodyBytes))
	} else {
		apiResp.Response = string(bodyBytes)
	}

	return apiResp, nil
}

// callPUT helper for PUT requests
func callPUT(url, token string, body interface{}) (ApiResponse, error) {
	apiResp := ApiResponse{}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return apiResp, fmt.Errorf("error marshaling JSON payload: %v", err)
	}

	resp, err := httpclient.SendRequestWithToken("PUT", url, token, bytes.NewBuffer(jsonData))
	if err != nil {
		return apiResp, fmt.Errorf("error making HTTP PUT request: %v", err)
	}
	defer resp.Body.Close()

	apiResp.ResponseCode = resp.StatusCode

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiResp, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode >= 400 {
		apiResp.Response = FormatErrorResponse(string(bodyBytes))
	} else {
		apiResp.Response = string(bodyBytes)
	}

	return apiResp, nil
}

// listPages GETs a list endpoint with the query and calls page with each body.
// It follows every page, unless the caller set a limit: then it is that one page.
func listPages(listURL, token string, queryParams map[string]string, linksKey string, page func(body []byte) error) error {
	query := url.Values{}
	for key, value := range queryParams {
		query.Set(key, value)
	}
	if len(query) > 0 {
		listURL += "?" + query.Encode()
	}
	if query.Has("limit") {
		linksKey = ""
	}
	return listAllPages(listURL, token, linksKey, page)
}

// listAllPages GETs a list URL and then every following page, calling page with each body.
// The next page is taken from the rel=next entry of the linksKey array; only its marker is
// used, because the href can carry an internal host or scheme.
func listAllPages(listURL, token, linksKey string, page func(body []byte) error) error {
	u, err := url.Parse(listURL)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %v", err)
	}

	seen := map[string]bool{}
	for {
		apiResp, err := callGET(u.String(), token)
		if err != nil {
			return err
		}
		if apiResp.ResponseCode != 200 {
			return fmt.Errorf("list request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
		}
		if err := page([]byte(apiResp.Response)); err != nil {
			return err
		}

		var envelope map[string]json.RawMessage
		if err := json.Unmarshal([]byte(apiResp.Response), &envelope); err != nil {
			return fmt.Errorf("failed to parse list response: %v", err)
		}
		var links []Link
		if raw, ok := envelope[linksKey]; ok {
			if err := json.Unmarshal(raw, &links); err != nil {
				return fmt.Errorf("failed to parse %s: %v", linksKey, err)
			}
		}
		marker := ""
		for _, link := range links {
			if link.Rel != "next" {
				continue
			}
			if next, err := url.Parse(link.Href); err == nil {
				marker = next.Query().Get("marker")
			}
		}
		if marker == "" || seen[marker] {
			return nil
		}
		seen[marker] = true

		query := u.Query()
		query.Set("marker", marker)
		u.RawQuery = query.Encode()
	}
}

func isUuid(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
