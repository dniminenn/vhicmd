package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/facette/natsort"
	"github.com/jessegalley/vhicmd/api"
	"github.com/jessegalley/vhicmd/internal/responseparser"
	"github.com/spf13/cobra"
)

var (
	flagSnapshotVolume      string
	flagSnapshotName        string
	flagSnapshotDescription string
	flagSnapshotForce       bool
)

var listSnapshotsCmd = &cobra.Command{
	Use:     "snapshots",
	Aliases: []string{"snap", "snaps", "snapshot"},
	Short:   "List volume snapshots",
	RunE: func(cmd *cobra.Command, args []string) error {
		storageURL, err := validateTokenEndpoint(tok, "volumev3")
		if err != nil {
			return err
		}

		queryParams := make(map[string]string)
		if flagSnapshotVolume != "" {
			volumeID, err := api.GetVolumeIDByName(storageURL, tok.Value, flagSnapshotVolume)
			if err != nil {
				return err
			}
			queryParams["volume_id"] = volumeID
		}

		resp, err := api.ListSnapshots(storageURL, tok.Value, queryParams)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(resp.Snapshots, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		sort.Slice(resp.Snapshots, func(i, j int) bool {
			return natsort.Compare(resp.Snapshots[i].Name, resp.Snapshots[j].Name)
		})
		var rows [][]string
		for _, s := range resp.Snapshots {
			rows = append(rows, []string{
				responseparser.Name(s.Name),
				s.ID,
				s.VolumeID,
				fmt.Sprintf("%d GB", s.Size),
				responseparser.VolumeStatus(s.Status),
				s.CreatedAt,
			})
		}
		responseparser.PrintTable([]string{"NAME", "ID", "VOLUME ID", "SIZE", "STATUS", "CREATED"}, rows)
		return nil
	},
}

var snapshotDetailsCmd = &cobra.Command{
	Use:     "snapshot [snapshot_id]",
	Aliases: []string{"snap"},
	Short:   "Show details of a specific volume snapshot",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storageURL, err := validateTokenEndpoint(tok, "volumev3")
		if err != nil {
			return err
		}

		snapshotID, err := api.GetSnapshotIDByName(storageURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		s, err := api.GetSnapshotDetails(storageURL, tok.Value, snapshotID)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(s, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		rows := [][]string{
			{"ID", s.ID},
			{"Name", responseparser.Name(s.Name)},
			{"Status", responseparser.VolumeStatus(s.Status)},
			{"Size", fmt.Sprintf("%d GB", s.Size)},
			{"Volume ID", s.VolumeID},
			{"Created", s.CreatedAt},
		}
		if s.UpdatedAt != "" {
			rows = append(rows, []string{"Updated", s.UpdatedAt})
		}
		if s.Description != "" {
			rows = append(rows, []string{"Description", s.Description})
		}
		if s.Progress != "" {
			rows = append(rows, []string{"Progress", s.Progress})
		}
		responseparser.PrintFieldTable("Snapshot Info", rows)

		if len(s.Metadata) > 0 {
			keys := make([]string, 0, len(s.Metadata))
			for k := range s.Metadata {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var meta [][]string
			for _, k := range keys {
				meta = append(meta, []string{k, s.Metadata[k]})
			}
			fmt.Println("\nMetadata:")
			responseparser.PrintTable([]string{"Key", "Value"}, meta)
		}
		return nil
	},
}

var createSnapshotCmd = &cobra.Command{
	Use:     "snapshot <volume-id|volume-name>",
	Aliases: []string{"snap"},
	Short:   "Create a snapshot of a volume",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storageURL, err := validateTokenEndpoint(tok, "volumev3")
		if err != nil {
			return err
		}

		volumeID, err := api.GetVolumeIDByName(storageURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		var request api.CreateSnapshotRequest
		request.Snapshot.VolumeID = volumeID
		request.Snapshot.Name = flagSnapshotName
		request.Snapshot.Description = flagSnapshotDescription
		request.Snapshot.Force = flagSnapshotForce

		s, err := api.CreateSnapshot(storageURL, tok.Value, request)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(s, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		fmt.Printf("Snapshot created: ID: %s, Name: %s, Volume: %s, Status: %s\n", s.ID, s.Name, s.VolumeID, s.Status)
		return nil
	},
}

var deleteSnapshotCmd = &cobra.Command{
	Use:     "snapshot <snapshot_id>",
	Aliases: []string{"snap"},
	Short:   "Delete a volume snapshot",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storageURL, err := validateTokenEndpoint(tok, "volumev3")
		if err != nil {
			return err
		}

		snapshotID, err := api.GetSnapshotIDByName(storageURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		if err := api.DeleteSnapshot(storageURL, tok.Value, snapshotID); err != nil {
			return err
		}

		fmt.Printf("Snapshot %s deleted\n", snapshotID)
		return nil
	},
}

func init() {
	listSnapshotsCmd.Flags().StringVar(&flagSnapshotVolume, "volume", "", "Only snapshots of this volume (ID or name)")

	createSnapshotCmd.Flags().StringVar(&flagSnapshotName, "name", "", "Name of the snapshot")
	createSnapshotCmd.Flags().StringVar(&flagSnapshotDescription, "description", "", "Description of the snapshot")
	createSnapshotCmd.Flags().BoolVar(&flagSnapshotForce, "force", false, "Snapshot the volume even when it is in use")
	createSnapshotCmd.MarkFlagRequired("name")

	listCmd.AddCommand(listSnapshotsCmd)
	detailsCmd.AddCommand(snapshotDetailsCmd)
	createCmd.AddCommand(createSnapshotCmd)
	deleteCmd.AddCommand(deleteSnapshotCmd)
}
