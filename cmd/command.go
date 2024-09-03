/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/dnstapir/tapir"
	"github.com/miekg/dns"
	"github.com/ryanuber/columnize"
	"github.com/spf13/cobra"
)

var BumpCmd = &cobra.Command{
	Use:   "bump",
	Short: "Instruct TAPIR-POP to bump the SOA serial of the RPZ zone",
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendCommandCmd(tapir.CommandPost{
			Command: "bump",
			Zone:    dns.Fqdn(tapir.GlobalCF.Zone),
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		fmt.Printf("%s\n", resp.Msg)
	},
}

var PopCmd = &cobra.Command{
	Use:   "pop",
	Short: "Prefix command to TAPIR-POP, only usable via sub-commands",
}

var PopMqttCmd = &cobra.Command{
	Use:   "mqtt",
	Short: "Prefix command to TAPIR-POP MQTT, only usable via sub-commands",
}

var PopStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get the status of TAPIR-POP",
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendCommandCmd(tapir.CommandPost{
			Command: "status",
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		fmt.Printf("%s\n", resp.Msg)

		fmt.Printf("TAPIR-POP Status: %+v\n", resp.TapirFunctionStatus)
		if len(resp.TapirFunctionStatus.ComponentStatus) != 0 {
			tfs := resp.TapirFunctionStatus
			fmt.Printf("TAPIR-POP Status. Reported components: %d Total errors (since last start): %d\n", len(tfs.ComponentStatus), tfs.NumFailures)
			var out = []string{"Component|Status|Error msg|# Fails|# Warns|LastFailure|LastSuccess"}
			for k, v := range tfs.ComponentStatus {
				out = append(out, fmt.Sprintf("%s|%s|%s|%d|%d|%v|%v", k, v.Status, v.ErrorMsg, v.NumFails, v.NumWarns, v.LastFail.Format(tapir.TimeLayout), v.LastSuccess.Format(tapir.TimeLayout)))
			}
			fmt.Printf("%s\n", columnize.SimpleFormat(out))
		}
	},
}

var PopStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Instruct TAPIR-POP to stop",
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendCommandCmd(tapir.CommandPost{
			Command: "stop",
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		fmt.Printf("%s\n", resp.Msg)
	},
}

var PopMqttStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Instruct TAPIR-POP MQTT Engine to start",
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendCommandCmd(tapir.CommandPost{
			Command: "mqtt-start",
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		fmt.Printf("%s\n", resp.Msg)
	},
}

var PopMqttStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Instruct TAPIR-POP MQTT Engine to stop",
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendCommandCmd(tapir.CommandPost{
			Command: "mqtt-stop",
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		fmt.Printf("%s\n", resp.Msg)
	},
}

var PopMqttRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Instruct TAPIR-POP MQTT Engine to restart",
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendCommandCmd(tapir.CommandPost{
			Command: "mqtt-restart",
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		fmt.Printf("%s\n", resp.Msg)
	},
}

func init() {
	rootCmd.AddCommand(BumpCmd, PopCmd)
	PopCmd.AddCommand(PopStatusCmd, PopStopCmd, PopMqttCmd)
	PopMqttCmd.AddCommand(PopMqttStartCmd, PopMqttStopCmd, PopMqttRestartCmd)

	BumpCmd.Flags().StringVarP(&tapir.GlobalCF.Zone, "zone", "z", "", "Zone name")
}

func SendCommandCmd(data tapir.CommandPost) tapir.CommandResponse {
	_, buf, _ := api.RequestNG(http.MethodPost, "/command", data, true)

	var cr tapir.CommandResponse

	err := json.Unmarshal(buf, &cr)
	if err != nil {
		log.Fatalf("Error from json.Unmarshal: %v\n", err)
	}
	return cr
}
