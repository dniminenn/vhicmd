package cmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "List compute hosts and their details",
	RunE: func(cmd *cobra.Command, args []string) error {
		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}

		result, err := api.ListHosts(computeURL, tok.Value)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(result.Hosts, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		for _, host := range result.Hosts {
			fmt.Printf("Host: %s\n", host.HostName)
			fmt.Printf("  Service: %s\n", host.Service)
			fmt.Printf("  Zone: %s\n", host.Zone)
			fmt.Printf("  Status: %s\n", host.Status)
			fmt.Printf("  State: %s\n", host.State)
			fmt.Printf("  Updated: %s\n", host.Updated)

			if len(host.Resources) > 0 {
				fmt.Printf("  Resources:\n")
				keys := make([]string, 0, len(host.Resources))
				for k := range host.Resources {
					keys = append(keys, k)
				}
				sort.Strings(keys)

				for _, k := range keys {
					v := host.Resources[k]
					val := fmt.Sprintf("%v", v)
					if reflect.TypeOf(v).Kind() == reflect.Float64 {
						val = fmt.Sprintf("%.2f", v)
					}
					fmt.Printf("    %s: %s\n", formatKey(k), val)
				}
			}
			fmt.Println()
		}

		return nil
	},
}

func formatKey(key string) string {
	parts := strings.Split(key, "_")
	c := cases.Title(language.English)
	for i, part := range parts {
		parts[i] = c.String(part)
	}
	return strings.Join(parts, " ")
}

func init() {
	rootCmd.AddCommand(hostsCmd)
	hostsCmd.Flags().BoolVar(&flagJsonOutput, "json", false, "Output in JSON format")
}
