package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Snapshot represents a cinder volume snapshot.
type Snapshot struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Size        int               `json:"size"`
	VolumeID    string            `json:"volume_id"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	Progress    string            `json:"os-extended-snapshot-attributes:progress,omitempty"`
	ProjectID   string            `json:"os-extended-snapshot-attributes:project_id,omitempty"`
	Metadata    map[string]string `json:"metadata"`
}

// SnapshotListResponse represents the response for listing snapshots.
type SnapshotListResponse struct {
	Snapshots []Snapshot `json:"snapshots"`
}

// CreateSnapshotRequest represents the payload for creating a snapshot.
// Force is needed to snapshot an in-use volume.
type CreateSnapshotRequest struct {
	Snapshot struct {
		VolumeID    string `json:"volume_id"`
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
		Force       bool   `json:"force,omitempty"`
	} `json:"snapshot"`
}

// ListSnapshots fetches all snapshots matching the query (volume_id, name, ...), every page.
func ListSnapshots(storageURL, token string, queryParams map[string]string) (SnapshotListResponse, error) {
	var result SnapshotListResponse
	result.Snapshots = []Snapshot{}

	query := url.Values{}
	for key, value := range queryParams {
		query.Set(key, value)
	}
	listURL := fmt.Sprintf("%s/snapshots/detail", storageURL)
	if len(query) > 0 {
		listURL += "?" + query.Encode()
	}

	err := listAllPages(listURL, token, "snapshots_links", func(body []byte) error {
		var page SnapshotListResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("failed to parse snapshots response: %v", err)
		}
		result.Snapshots = append(result.Snapshots, page.Snapshots...)
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("failed to list snapshots: %v", err)
	}
	return result, nil
}

// GetSnapshotDetails fetches one snapshot by ID.
func GetSnapshotDetails(storageURL, token, snapshotID string) (Snapshot, error) {
	var wrapper struct {
		Snapshot Snapshot `json:"snapshot"`
	}

	url := fmt.Sprintf("%s/snapshots/%s", storageURL, snapshotID)

	apiResp, err := callGET(url, token)
	if err != nil {
		return wrapper.Snapshot, fmt.Errorf("failed to fetch snapshot details: %v", err)
	}
	if apiResp.ResponseCode != 200 {
		return wrapper.Snapshot, fmt.Errorf("get snapshot details request failed [%d]: %s",
			apiResp.ResponseCode, apiResp.Response)
	}

	if err := json.Unmarshal([]byte(apiResp.Response), &wrapper); err != nil {
		return wrapper.Snapshot, fmt.Errorf("failed to parse snapshot details: %v", err)
	}
	return wrapper.Snapshot, nil
}

// CreateSnapshot sends a request to snapshot a volume.
func CreateSnapshot(storageURL, token string, request CreateSnapshotRequest) (Snapshot, error) {
	var wrapper struct {
		Snapshot Snapshot `json:"snapshot"`
	}

	url := fmt.Sprintf("%s/snapshots", storageURL)

	apiResp, err := callPOST(url, token, request)
	if err != nil {
		return wrapper.Snapshot, fmt.Errorf("failed to create snapshot: %v", err)
	}
	if apiResp.ResponseCode != 202 {
		return wrapper.Snapshot, fmt.Errorf("snapshot creation request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	if err := json.Unmarshal([]byte(apiResp.Response), &wrapper); err != nil {
		return wrapper.Snapshot, fmt.Errorf("failed to parse snapshot creation response: %v", err)
	}
	return wrapper.Snapshot, nil
}

// DeleteSnapshot sends a request to delete a snapshot.
func DeleteSnapshot(storageURL, token, snapshotID string) error {
	url := fmt.Sprintf("%s/snapshots/%s", storageURL, snapshotID)

	resp, err := callDELETE(url, token)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %v", err)
	}
	if resp.ResponseCode != 202 && resp.ResponseCode != 204 {
		return fmt.Errorf("failed to delete snapshot [%d]: %s", resp.ResponseCode, resp.Response)
	}
	return nil
}

// GetSnapshotIDByName returns a UUID as is, or the ID of the one snapshot with that exact name.
func GetSnapshotIDByName(storageURL, token, snapshotName string) (string, error) {
	if isUuid(snapshotName) {
		return snapshotName, nil
	}

	snapshots, err := ListSnapshots(storageURL, token, map[string]string{"name": snapshotName})
	if err != nil {
		return "", err
	}

	var found []Snapshot
	for _, snapshot := range snapshots.Snapshots {
		if snapshot.Name == snapshotName {
			found = append(found, snapshot)
		}
	}

	if len(found) == 0 {
		return "", fmt.Errorf("no snapshots found for name %s", snapshotName)
	}
	if len(found) > 1 {
		return "", fmt.Errorf("multiple snapshots found for name %s", snapshotName)
	}
	return found[0].ID, nil
}
