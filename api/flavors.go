package api

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Flavor is a flavor as the detail calls return it.
type Flavor struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"` // microversion>=2.55
	VCPUs       int               `json:"vcpus"`
	RAM         int               `json:"ram"`  // MiB
	Disk        int               `json:"disk"` // GiB; 0 when the VM boots from a volume
	Ephemeral   int               `json:"OS-FLV-EXT-DATA:ephemeral"`
	Swap        FlavorSwap        `json:"swap"`
	RxTxFactor  float64           `json:"rxtx_factor"`
	IsPublic    bool              `json:"os-flavor-access:is_public"`
	IsDisabled  bool              `json:"OS-FLV-DISABLED:disabled"`
	ExtraSpecs  map[string]string `json:"extra_specs,omitempty"` // microversion>=2.61
	Links       []struct {
		Href string `json:"href"`
		Rel  string `json:"rel"`
	} `json:"links"`
}

// FlavorSwap is a flavor's swap in MiB. Nova sends "" for none and a number otherwise.
type FlavorSwap int

func (s *FlavorSwap) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*s = FlavorSwap(n)
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf("flavor swap %s is neither a number nor a string", data)
	}
	if str == "" {
		*s = 0
		return nil
	}
	n, err := strconv.Atoi(str)
	if err != nil {
		return fmt.Errorf("flavor swap %q is not a number", str)
	}
	*s = FlavorSwap(n)
	return nil
}

// FlavorListResponse represents the response for listing flavors.
type FlavorListResponse struct {
	Flavors []Flavor `json:"flavors"`
}

// ListFlavors fetches the flavors with their sizes, every page unless a limit or marker is set.
func ListFlavors(computeURL, token string, queryParams map[string]string) (FlavorListResponse, error) {
	result := FlavorListResponse{Flavors: []Flavor{}}

	listURL := fmt.Sprintf("%s/flavors/detail", computeURL)
	err := listPages(listURL, token, queryParams, "flavors_links", func(body []byte) error {
		var page FlavorListResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("failed to parse flavors response: %v", err)
		}
		result.Flavors = append(result.Flavors, page.Flavors...)
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("failed to fetch flavors: %v", err)
	}
	return result, nil
}

// GetFlavorDetails fetches a single flavor by ID.
func GetFlavorDetails(computeURL, token, flavorID string) (Flavor, error) {
	var result struct {
		Flavor Flavor `json:"flavor"`
	}

	url := fmt.Sprintf("%s/flavors/%s", computeURL, flavorID)
	apiResp, err := callGET(url, token)
	if err != nil {
		return result.Flavor, fmt.Errorf("failed to GET flavor: %v", err)
	}
	if apiResp.ResponseCode != 200 {
		return result.Flavor, fmt.Errorf("flavor details request failed [%d]: %s",
			apiResp.ResponseCode, apiResp.Response)
	}
	err = json.Unmarshal([]byte(apiResp.Response), &result)
	if err != nil {
		return result.Flavor, fmt.Errorf("failed to parse flavor details: %v", err)
	}
	return result.Flavor, nil
}

// The flavor names are not unique, so this function a single flavor if only one is found,
// if multiple flavors or none are found, it returns an error.
// func GetFlavorIDByName(computeURL, token, flavorName string) (string, error) {
// 	if isUuid(flavorName) {
// 		return flavorName, nil
// 	}

// 	flavors, err := ListFlavors(computeURL, token, nil)

// 	if err != nil {
// 		return "", err
// 	}

// 	var foundFlavors []Flavor

// 	for _, flavor := range flavors.Flavors {
// 		if strings.Contains(flavor.Name, flavorName) {
// 			foundFlavors = append(foundFlavors, flavor)
// 		}
// 	}

// 	if len(foundFlavors) == 0 {
// 		return "", fmt.Errorf("no flavors found for name %s", flavorName)
// 	}

// 	if len(foundFlavors) > 1 {
// 		return "", fmt.Errorf("multiple flavors found for name %s", flavorName)
// 	}

// 	return foundFlavors[0].ID, nil
// }

func GetFlavorIDByName(computeURL, token, flavorName string) (string, error) {
	if isUuid(flavorName) {
		return flavorName, nil
	}

	flavors, err := ListFlavors(computeURL, token, nil)

	if err != nil {
		return "", err
	}

	var foundFlavors []Flavor

	for _, flavor := range flavors.Flavors {
		if flavor.Name == flavorName {
			foundFlavors = append(foundFlavors, flavor)
		}
	}

	if len(foundFlavors) == 0 {
		return "", fmt.Errorf("no flavors found for name %s", flavorName)
	}

	if len(foundFlavors) > 1 {
		return "", fmt.Errorf("multiple flavors found for name %s", flavorName)
	}

	return foundFlavors[0].ID, nil
}
