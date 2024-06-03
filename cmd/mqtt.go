/*
 * Copyright (c) DNS TAPIR
 */
package cmd

import (
	"bufio"
	"bytes"
	"strconv"

	//	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dnstapir/tapir"
	"github.com/miekg/dns"
	"github.com/ryanuber/columnize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var mqttclientid string
var mqttpub, mqttsub bool

// var testMsg = tapir.TapirMsg{
//	MsgType: "observation",
//	Added: []tapir.Domain{
//		tapir.Domain{
//			Name: "frobozz.com.",
//			Tags: []string{"new", "high-volume", "bad-ip"},
//		},
//		tapir.Domain{
//			Name: "johani.org.",
//			Tags: []string{"old", "low-volume", "good-ip"},
//		},
//	},
//	Removed: []tapir.Domain{
//		tapir.Domain{
//			Name: "dnstapir.se.",
//		},
//	},
// }

var mqttCmd = &cobra.Command{
	Use:   "mqtt",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example: to quickly create a Cobra application.`,
}

var mqttEngineCmd = &cobra.Command{
	Use:   "engine",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example: to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup

		var pubsub uint8
		if mqttpub {
			pubsub = pubsub | tapir.TapirPub
		}
		if mqttsub {
			pubsub = pubsub | tapir.TapirSub
		}

		meng, err := tapir.NewMqttEngine(mqttclientid, pubsub, log.Default())
		if err != nil {
			fmt.Printf("Error from NewMqttEngine: %v\n", err)
			os.Exit(1)
		}

		cmnder, outbox, inbox, err := meng.StartEngine()
		if err != nil {
			log.Fatalf("Error from StartEngine(): %v", err)
		}

		stdin := bufio.NewReader(os.Stdin)
		count := 0
		buf := new(bytes.Buffer)

		SetupInterruptHandler(cmnder)

		if mqttsub {
			wg.Add(1)
			SetupSubPrinter(inbox)
		}

		srcname := viper.GetString("mqtt.tapir.srcname")
		if srcname == "" {
			fmt.Printf("Error: mqtt.tapir.srcname not specified in config")
			os.Exit(1)
		}

		if mqttpub {
			for {
				count++
				msg, err := stdin.ReadString('\n')
				if err == io.EOF {
					os.Exit(0)
				}
				fmt.Printf("Read: %s", msg)
				msg = tapir.Chomp(msg)
				if len(msg) == 0 {
					fmt.Printf("Empty message ignored.\n")
					continue
				}
				if strings.ToUpper(msg) == "QUIT" {
					wg.Done()
					break
				}

				buf.Reset()
				outbox <- tapir.MqttPkg{
					Type: "data",
					Data: tapir.TapirMsg{
						Msg:       msg,
						SrcName:   srcname,
						TimeStamp: time.Now(),
					},
				}
			}
			respch := make(chan tapir.MqttEngineResponse, 2)
			meng.CmdChan <- tapir.MqttEngineCmd{Cmd: "stop", Resp: respch}
			// var r tapir.MqttEngineResponse
			r := <-respch
			fmt.Printf("Response from MQTT Engine: %v\n", r)
		}
		wg.Wait()
	},
}

var mqttIntelUpdateCmd = &cobra.Command{
	Use:   "observation",
	Short: "Send observations in TapirMsg form to the tapir intel MQTT topic (debug tool)",
	Long: `Will query for operation (add|del), domain name and tags.
Will end the loop on the operation (or domain name) "QUIT"`,
	Run: func(cmd *cobra.Command, args []string) {
		meng, err := tapir.NewMqttEngine(mqttclientid, tapir.TapirPub, log.Default()) // pub, no sub
		if err != nil {
			fmt.Printf("Error from NewMqttEngine: %v\n", err)
			os.Exit(1)
		}

		topic := viper.GetString("mqtt.topic")
		valkey, err := tapir.FetchMqttValidatorKey(topic, viper.GetString("mqtt.validatorkey"))
		if err != nil {
			log.Fatalf("Error fetching MQTT validator key: %v", err)
		}
		meng.AddTopic(topic, valkey)

		cmnder, outbox, _, err := meng.StartEngine()
		if err != nil {
			log.Fatalf("Error from StartEngine(): %v", err)
		}

		count := 0

		SetupInterruptHandler(cmnder)

		srcname := viper.GetString("mqtt.tapir.srcname")
		if srcname == "" {
			fmt.Printf("Error: mqtt.tapir.srcname not specified in config")
			os.Exit(1)
		}

		var op, names, tags string
		var tmsg = tapir.TapirMsg{
			SrcName:   srcname,
			Creator:   "tapir-cli",
			MsgType:   "observation",
			ListType:  "greylist",
			TimeStamp: time.Now(),
		}

		var snames []string
		var tagmask tapir.TagMask

		var ops = []string{"add", "del", "show", "send", "set-ttl", "list-tags", "quit"}
		fmt.Printf("Defined operations are: %v\n", ops)

		var tds []tapir.Domain
		// var ttl time.Duration = 60 * time.Second
		var ttl int = 60

	cmdloop:
		for {
			count++
			op = TtyRadioButtonQ("Operation", "add", ops)
			switch op {
			case "quit":
				fmt.Println("QUIT cmd recieved.")
				break cmdloop

			case "set-ttl":
				ttl = TtyIntQuestion("TTL (in seconds)", 60, false)
				// fmt.Printf("TTL: got: %d\n", tmp)
				// ttl = time.Duration(tmp) * time.Second
				// fmt.Printf("TTL: got: %d ttl: %v\n", tmp, ttl)
			case "add", "del":
				names = TtyQuestion("Domain names", names, false)
				snames = strings.Fields(names)
				if len(snames) > 0 && strings.ToUpper(snames[0]) == "QUIT" {
					break cmdloop
				}

				if op == "add" {
				retry:
					for {
						tags = TtyQuestion("Tags", tags, false)
						tagmask, err = tapir.StringsToTagMask(strings.Fields(tags))
						if err != nil {
							fmt.Printf("Error from StringsToTagMask: %v\n", err)
							fmt.Printf("Defined tags are: %v\n", tapir.DefinedTags)
							continue retry
						}
						break
					}
					if tapir.GlobalCF.Verbose {
						fmt.Printf("TagMask: %032b\n", tagmask)
					}
				}
				for _, name := range snames {
					tds = append(tds, tapir.Domain{
						Name:      dns.Fqdn(name),
						TimeAdded: time.Now(),
						TTL:       ttl,
						TagMask:   tagmask,
					})
				}

				if op == "add" {
					tmsg.Added = append(tmsg.Added, tds...)
					tmsg.Msg = "it is greater to give than to take"
				} else {
					tmsg.Removed = append(tmsg.Removed, tds...)
					tmsg.Msg = "happiness is a negative diff"
				}
				tds = []tapir.Domain{}

			case "show":
				var out = []string{"Domain|Tags"}
				for _, td := range tmsg.Added {
					out = append(out, fmt.Sprintf("ADD: %s|%032b", td.Name, td.TagMask))
				}
				for _, td := range tmsg.Removed {
					out = append(out, fmt.Sprintf("DEL: %s", td.Name))
				}
				fmt.Println(columnize.SimpleFormat(out))

			case "list-tags":
				var out = []string{"Name|Bit"}
				var tagmask tapir.TagMask
				for _, t := range tapir.DefinedTags {
					tagmask, _ = tapir.StringsToTagMask([]string{t})
					out = append(out, fmt.Sprintf("%s|%032b", t, tagmask))
				}
				fmt.Println(columnize.SimpleFormat(out))

			case "send":
				outbox <- tapir.MqttPkg{
					Type: "data",
					Data: tmsg,
				}

				tmsg = tapir.TapirMsg{
					SrcName:   srcname,
					Creator:   "tapir-cli",
					MsgType:   "observation",
					ListType:  "greylist",
					TimeStamp: time.Now(),
				}
				tds = []tapir.Domain{}
			}
		}
		respch := make(chan tapir.MqttEngineResponse, 2)
		meng.CmdChan <- tapir.MqttEngineCmd{Cmd: "stop", Resp: respch}
		r := <-respch
		fmt.Printf("Response from MQTT Engine: %v\n", r)
	},
}

func init() {
	rootCmd.AddCommand(mqttCmd)
	mqttCmd.AddCommand(mqttEngineCmd, mqttIntelUpdateCmd)

	mqttCmd.PersistentFlags().StringVarP(&mqttclientid, "clientid", "", "",
		"MQTT client id, must be unique")
	mqttEngineCmd.Flags().BoolVarP(&mqttpub, "pub", "", false, "Enable pub support")
	mqttEngineCmd.Flags().BoolVarP(&mqttsub, "sub", "", false, "Enable sub support")
}

func SetupSubPrinter(inbox chan tapir.MqttPkg) {
	go func() {
		var pkg tapir.MqttPkg
		for {
			select {
			case pkg = <-inbox:
				var out []string
				fmt.Printf("Received TAPIR MQTT Message:\n")
				for _, a := range pkg.Data.Added {
					out = append(out, fmt.Sprintf("ADD: %s|%032b", a.Name, a.TagMask))
				}
				for _, a := range pkg.Data.Removed {
					out = append(out, fmt.Sprintf("DEL: %s", a.Name))
				}
				fmt.Println(columnize.SimpleFormat(out))
			}
		}
	}()
}

func SetupInterruptHandler(cmnder chan tapir.MqttEngineCmd) {
	respch := make(chan tapir.MqttEngineResponse, 2)

	ic := make(chan os.Signal, 1)
	signal.Notify(ic, os.Interrupt, syscall.SIGTERM)
	go func() {
		for {
			select {

			case <-ic:
				fmt.Println("SIGTERM interrupt received, sending stop signal to MQTT Engine")
				cmnder <- tapir.MqttEngineCmd{Cmd: "stop", Resp: respch}
				r := <-respch
				if r.Error {
					fmt.Printf("Error: %s\n", r.ErrorMsg)
				} else {
					fmt.Printf("MQTT Engine: %s\n", r.Status)
				}
				os.Exit(1)
			}
		}
	}()
}

func TtyQuestion(query, oldval string, force bool) string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [%s]: ", query, oldval)
		text, _ := reader.ReadString('\n')
		if text == "\n" {
			fmt.Printf("[empty response, keeping previous value]\n")
			if oldval != "" {
				return oldval // all ok
			} else if force {
				fmt.Printf("[error: previous value was empty string, not allowed]\n")
				continue
			}
			return oldval
		} else {
			// regardless of force we accept non-empty response
			return strings.TrimSuffix(text, "\n")
		}
	}
}

func TtyIntQuestion(query string, oldval int, force bool) int {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [%d]: ", query, oldval)
		text, _ := reader.ReadString('\n')
		if text == "\n" {
			fmt.Printf("[empty response, keeping previous value]\n")
			if oldval != 0 {
				return oldval // all ok
			} else if force {
				fmt.Printf("[error: previous value was empty string, not allowed]\n")
				continue
			}
			return oldval
		} else {
			text = tapir.Chomp(text)
			// regardless of force we accept non-empty response
			val, err := strconv.Atoi(text)
			if err != nil {
				fmt.Printf("Error: %s is not an integer\n", text)
				continue
			}
			return val
		}
	}
}

func TtyYesNo(query, defval string) string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [%s]: ", query, defval)
		text, _ := reader.ReadString('\n')
		if text == "\n" {
			if defval != "" {
				fmt.Printf("[empty response, using default value]\n")
				return defval // all ok
			}
			fmt.Printf("[error: default value is empty string, not allowed]\n")
			continue
		} else {
			val := strings.ToLower(strings.TrimSuffix(text, "\n"))
			if (val == "yes") || (val == "no") {
				return val
			}
			fmt.Printf("Answer '%s' not accepted. Only yes or no.\n", val)
		}
	}
}

func TtyRadioButtonQ(query, defval string, choices []string) string {
	var C []string
	for _, c := range choices {
		C = append(C, strings.ToLower(c))
	}

	allowed := func(str string) bool {
		for _, c := range C {
			if str == c {
				return true
			}
		}
		return false
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [%s]: ", query, defval)
		text, _ := reader.ReadString('\n')
		if text == "\n" {
			if defval != "" {
				fmt.Printf("[empty response, using default value]\n")
				return defval // all ok
			}
			fmt.Printf("[error: default value is empty string, not allowed]\n")
			continue
		} else {
			val := strings.ToLower(strings.TrimSuffix(text, "\n"))
			if allowed(val) {
				return val
			}
			fmt.Printf("Answer '%s' not accepted. Possible choices are: %v\n", val, choices)
		}
	}
}
