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
	flagFloatingIPNetwork string
	flagFloatingIPPort    string
	flagFloatingIPDetach  bool
)

var listFloatingIPsCmd = &cobra.Command{
	Use:     "floating-ips",
	Aliases: []string{"fip", "fips", "floatingips", "floating-ip"},
	Short:   "List floating IPs",
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		resp, err := api.ListFloatingIPs(networkURL, tok.Value, nil)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(resp.FloatingIPs, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		sort.Slice(resp.FloatingIPs, func(i, j int) bool {
			return natsort.Compare(resp.FloatingIPs[i].FloatingIPAddress, resp.FloatingIPs[j].FloatingIPAddress)
		})
		var rows [][]string
		for _, f := range resp.FloatingIPs {
			rows = append(rows, []string{
				responseparser.Name(f.FloatingIPAddress),
				f.ID,
				stringOrNone(f.FixedIPAddress),
				stringOrNone(f.PortID),
				f.Status,
				f.FloatingNetworkID,
			})
		}
		responseparser.PrintTable([]string{"FLOATING IP", "ID", "FIXED IP", "PORT ID", "STATUS", "NETWORK ID"}, rows)
		return nil
	},
}

var createFloatingIPCmd = &cobra.Command{
	Use:     "floating-ip",
	Aliases: []string{"fip", "floatingip"},
	Short:   "Allocate a floating IP from an external network",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		networkID, err := api.GetNetworkIDByName(networkURL, tok.Value, flagFloatingIPNetwork)
		if err != nil {
			return err
		}

		f, err := api.CreateFloatingIP(networkURL, tok.Value, networkID)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(f, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		fmt.Printf("Floating IP created: ID: %s, Address: %s, Network: %s\n", f.ID, f.FloatingIPAddress, f.FloatingNetworkID)
		return nil
	},
}

var deleteFloatingIPCmd = &cobra.Command{
	Use:     "floating-ip <id_or_address>",
	Aliases: []string{"fip", "floatingip"},
	Short:   "Release a floating IP",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		floatingIPID, err := api.GetFloatingIPIDByAddress(networkURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		if err := api.DeleteFloatingIP(networkURL, tok.Value, floatingIPID); err != nil {
			return err
		}

		fmt.Printf("Floating IP %s deleted\n", floatingIPID)
		return nil
	},
}

var updateFloatingIPCmd = &cobra.Command{
	Use:     "floating-ip <id_or_address>",
	Aliases: []string{"fip", "floatingip"},
	Short:   "Associate a floating IP with a port, or detach it",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if (flagFloatingIPPort == "") == !flagFloatingIPDetach {
			return fmt.Errorf("use exactly one of --port <port-id> or --detach")
		}

		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		floatingIPID, err := api.GetFloatingIPIDByAddress(networkURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		portID := ""
		if !flagFloatingIPDetach {
			portID, err = api.GetPortIDByName(networkURL, tok.Value, flagFloatingIPPort)
			if err != nil {
				return fmt.Errorf("failed to resolve port %q: %v", flagFloatingIPPort, err)
			}
		}

		f, err := api.SetFloatingIPPort(networkURL, tok.Value, floatingIPID, portID)
		if err != nil {
			return err
		}

		if flagFloatingIPDetach {
			fmt.Printf("Floating IP %s (%s) detached\n", f.FloatingIPAddress, floatingIPID)
		} else {
			fmt.Printf("Floating IP %s (%s) associated with port %s (fixed IP %s)\n",
				f.FloatingIPAddress, floatingIPID, portID, stringOrNone(f.FixedIPAddress))
		}
		return nil
	},
}

func init() {
	createFloatingIPCmd.Flags().StringVar(&flagFloatingIPNetwork, "network", "", "External network ID or name")
	createFloatingIPCmd.MarkFlagRequired("network")

	updateFloatingIPCmd.Flags().StringVar(&flagFloatingIPPort, "port", "", "Port ID or name to associate the floating IP with")
	updateFloatingIPCmd.Flags().BoolVar(&flagFloatingIPDetach, "detach", false, "Disassociate the floating IP from its port")

	listCmd.AddCommand(listFloatingIPsCmd)
	createCmd.AddCommand(createFloatingIPCmd)
	deleteCmd.AddCommand(deleteFloatingIPCmd)
	updateCmd.AddCommand(updateFloatingIPCmd)
}
