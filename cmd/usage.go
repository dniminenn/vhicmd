package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

const colorGreen = "\033[32m"
const colorReset = "\033[0m"

func formatRAMSize(mb float64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	bytes := int64(mb * float64(MB))
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	default:
		return fmt.Sprintf("%.2f MB", mb)
	}
}

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show resource usage statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}

		tenantID, _ := cmd.Flags().GetString("tenant-id")

		result, err := api.GetLimits(computeURL, tok.Value, true, tenantID)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		absolute := api.GetAbsoluteLimits(result.Limits)
		if absolute == nil {
			return fmt.Errorf("no absolute limits found in response")
		}

		projectName := tok.Project

		fmt.Printf("%s resource summary:\n", projectName)

		// Get specific metrics
		ram := absolute["totalRAMUsed"]
		cores := absolute["totalCoresUsed"]
		instances := absolute["totalInstancesUsed"]

		if ram != nil {
			fmt.Printf("RAM Used:\t\t%s%s%s\n", colorGreen, formatRAMSize(ram.(float64)), colorReset)
		}
		if cores != nil {
			fmt.Printf("Cores Used:\t\t%s%.0f%s\n", colorGreen, cores.(float64), colorReset)
		}
		if instances != nil {
			fmt.Printf("Instances Deployed:\t%s%.0f%s\n", colorGreen, instances.(float64), colorReset)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(usageCmd)
	usageCmd.Flags().String("tenant-id", "", "Show usage for specific tenant (admin only)")
	usageCmd.Flags().BoolVar(&flagJsonOutput, "json", false, "Output in JSON format")
}
