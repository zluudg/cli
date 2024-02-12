/*
 * Copyright (c) DNS TAPIR
 */
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/dnstapir/tapir"
	//	"github.com/ryanuber/columnize"
	"github.com/miekg/dns"
	"github.com/spf13/cobra"
)

var debugcmdCmd = &cobra.Command{
	Use:   "debugcmd",
	Short: "A brief description of your command",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("debugcmd called")

		var tm tapir.TagMask = 345
		fmt.Printf("%032b num tags: %d\n", tm, tm.NumTags())
	},
}

var debugZoneDataCmd = &cobra.Command{
	Use:   "zonedata",
	Short: "Return the ZoneData struct for the specified zone from server",
	Long: `Return the ZoneData struct from server
 (mostly useful with -d JSON prettyprinter).`,
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendDebugCmd(tapir.DebugPost{
			Command: "zonedata",
			Zone:    dns.Fqdn(tapir.GlobalCF.Zone),
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		//		zd := resp.ZoneData

		fmt.Printf("Received %d bytes of data\n", len(resp.Msg))
		//		fmt.Printf("Zone %s: RRs: %d Owners: %d\n", tapir.GlobalCF.Zone,
		//			len(zd.RRs), len(zd.Owners))
	},
}

var debugColourlistsCmd = &cobra.Command{
	Use:   "colourlists",
	Short: "Return the white/black/greylists from the current data structures",
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendDebugCmd(tapir.DebugPost{
			Command: "colourlists",
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		//		fmt.Printf("Received %d bytes of data\n", len(resp.Msg))

		for _, l := range resp.Lists["whitelist"] {
			fmt.Printf("white:%s\tcount=%d\tdesc=%s:\n\n%v\n\n", l.Name, len(l.Names), l.Description, l.Names)
		}
		for _, l := range resp.Lists["blacklist"] {
			fmt.Printf("black:%s\tcount=%d\tdesc=%s:\n\n%v\n\n", l.Name, len(l.Names), l.Description, l.Names)
		}
		for _, l := range resp.Lists["greylist"] {
			fmt.Printf("grey:%s\tcount=%d\tdesc=%s:\n\n%v\n\n", l.Name, len(l.Names), l.Description, l.Names)
		}
	},
}

var debugGenRpzCmd = &cobra.Command{
	Use:   "genrpz",
	Short: "Return the white/black/greylists from the current data structures",
	Run: func(cmd *cobra.Command, args []string) {
		resp := SendDebugCmd(tapir.DebugPost{
			Command: "gen-output",
		})
		if resp.Error {
			fmt.Printf("%s\n", resp.ErrorMsg)
		}

		fmt.Printf("Received %d bytes of data\n", len(resp.Msg))

		fmt.Printf("black count=%d: %v\n", resp.BlacklistedNames)
		fmt.Printf("grey count=%d: %v\n", resp.GreylistedNames)
		//	    	fmt.Printf("count=%d: %v\n", res.RpzOutput)
		for _, tn := range resp.RpzOutput {
			fmt.Printf("%s\n", (*tn.RR).String())
		}
	},
}

var zonefile string

var debugSyncZoneCmd = &cobra.Command{
	Use:   "synczone",
	Short: "A brief description of your command",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("synczone called")

		if tapir.GlobalCF.Zone == "" {
			fmt.Printf("Zone name not specified.\n")
			os.Exit(1)
		}

		if zonefile == "" {
			fmt.Printf("Zone file not specified.\n")
			os.Exit(1)
		}

		zd := tapir.ZoneData{
			ZoneType:   3, // zonetype=3 keeps RRs in a []OwnerData, with an OwnerIndex map[string]int to locate stuff
			ZoneName:   tapir.GlobalCF.Zone,
			RRKeepFunc: func(uint16) bool { return true },
			Logger:     log.Default(),
		}

		_, err := zd.ReadZoneFile(zonefile)
		if err != nil {
			log.Fatalf("ReloadAuthZones: Error from ReadZoneFile(%s): %v", zonefile, err)
		}

		// XXX: This will be wrong for zonetype=3 (which we're using)
		fmt.Printf("----- zd.BodyRRs: ----\n")
		tapir.PrintRRs(zd.BodyRRs)
		fmt.Printf("----- zd.RRs (pre-sync): ----\n")
		tapir.PrintRRs(zd.RRs)
		zd.Sync()
		fmt.Printf("----- zd.RRs (post-sync): ----\n")
		tapir.PrintRRs(zd.RRs)
		zd.Sync()
		fmt.Printf("----- zd.RRs (post-sync): ----\n")
		tapir.PrintRRs(zd.RRs)
		fmt.Printf("----- zd.BodyRRs: ----\n")
		tapir.PrintRRs(zd.BodyRRs)
	},
}

func init() {
	rootCmd.AddCommand(debugcmdCmd)
	debugcmdCmd.AddCommand(debugSyncZoneCmd, debugZoneDataCmd, debugColourlistsCmd, debugGenRpzCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// debugcmdCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	debugSyncZoneCmd.Flags().StringVarP(&tapir.GlobalCF.Zone, "zone", "z", "", "Zone name")
	debugZoneDataCmd.Flags().StringVarP(&tapir.GlobalCF.Zone, "zone", "z", "", "Zone name")
	debugSyncZoneCmd.Flags().StringVarP(&zonefile, "file", "f", "", "Zone file")
}

type DebugResponse struct {
	Msg      string
	Data     interface{}
	Error    bool
	ErrorMsg string
}

func SendDebugCmd(data tapir.DebugPost) tapir.DebugResponse {
	_, buf, _ := api.RequestNG(http.MethodPost, "/debug", data, true)

	var dr tapir.DebugResponse

	var pretty bytes.Buffer
	err := json.Indent(&pretty, buf, "", "   ")
	if err != nil {
		fmt.Printf("JSON parse error: %v", err)
	}
	//	fmt.Printf("Received %d bytes of data: %v\n", len(buf), pretty.String())
	//	os.Exit(1)

	err = json.Unmarshal(buf, &dr)
	if err != nil {
		log.Fatalf("Error from json.Unmarshal: %v\n", err)
	}
	return dr
}
