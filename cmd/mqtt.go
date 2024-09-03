/*
 * Copyright (c) DNS TAPIR
 */
package cmd

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"net/http"
	"path/filepath"

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

	"github.com/google/uuid"

	"github.com/dnstapir/tapir"
	"github.com/miekg/dns"
	"github.com/ryanuber/columnize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var mqttclientid, mqtttopic, defaulttopic, mqttgreylist, cfgfile string

var mqttfid string
var mqttpub, mqttsub, mqttretain, mqttconfigclear bool

var mqttCmd = &cobra.Command{
	Use:   "mqtt",
	Short: "Prefix command, not usable directly",
	Long:  `Prefix command, not usable directly.`,
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

		var statusch = make(chan tapir.ComponentStatusUpdate, 10)

		// If any status updates arrive, print them out
		go func() {
			for {
				select {
				case status := <-statusch:
					fmt.Printf("Status update: %+v\n", status)
				}
			}
		}()

		meng, err := tapir.NewMqttEngine("engine", mqttclientid, pubsub, statusch, log.Default())
		if err != nil {
			fmt.Printf("Error from NewMqttEngine: %v\n", err)
			os.Exit(1)
		}

		var canPub = true
		var canSub = true
		var signkey *ecdsa.PrivateKey
		var valkey *ecdsa.PublicKey

		switch mqtttopic {
		case "config":
			mqtttopic = viper.GetString("tapir.config.topic")
			signkey, err = tapir.FetchMqttSigningKey(mqtttopic, viper.GetString("tapir.config.signingkey"))
			if err != nil {
				fmt.Printf("Error fetching MQTT signing key: %s\n", viper.GetString("tapir.config.signingkey"))
				canPub = false
			}
			valkey, err = tapir.FetchMqttValidatorKey(mqtttopic, viper.GetString("tapir.config.validatorkey"))
			if err != nil {
				fmt.Printf("Error fetching MQTT signing key: %s\n", viper.GetString("tapir.config.validatorkey"))
				canSub = false
			}

		case "observations":
			mqtttopic = viper.GetString("tapir.observations.topic")
			signkey, err = tapir.FetchMqttSigningKey(mqtttopic, viper.GetString("tapir.observations.signingkey"))
			if err != nil {
				fmt.Printf("Error fetching MQTT signing key: %s\n", viper.GetString("tapir.observations.signingkey"))
				canPub = false
			}
			valkey, err = tapir.FetchMqttValidatorKey(mqtttopic, viper.GetString("tapir.observations.validatorkey"))
			if err != nil {
				fmt.Printf("Error fetching MQTT signing key: %s\n", viper.GetString("tapir.observations.validatorkey"))
				canSub = false
			}

		case "status":
			mqtttopic = viper.GetString("tapir.status.topic")
			signkey, err = tapir.FetchMqttSigningKey(mqtttopic, viper.GetString("tapir.status.signingkey"))
			if err != nil {
				fmt.Printf("Error fetching MQTT signing key: %s\n", viper.GetString("tapir.status.signingkey"))
				canPub = false
			}
			valkey, err = tapir.FetchMqttValidatorKey(mqtttopic, viper.GetString("tapir.status.validatorkey"))
			if err != nil {
				fmt.Printf("Error fetching MQTT signing key: %s\n", viper.GetString("tapir.status.validatorkey"))
				canSub = false
			}

		default:
			log.Fatalf("Invalid MQTT topic: %s (must be config or observations)", mqtttopic)
		}

		if canPub || canSub {
			fmt.Printf("Adding topic: %s\n", mqtttopic)
			// meng.AddTopic(mqtttopic, signkey, valkey)
			meng.PubSubToTopic(mqtttopic, signkey, valkey, nil)
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

		srcname := viper.GetString("tapir.observations.srcname")
		if srcname == "" {
			fmt.Printf("Error: tapir.observations.srcname not specified in config")
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

var mqttTapirCmd = &cobra.Command{
	Use:   "tapir",
	Short: "Empty prefix command only useable via sub-commands",
}

type ConfigFoo struct {
	GlobalConfig tapir.GlobalConfig
}

var mqttTapirConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Send TAPIR-POP global config in TapirMsg form to the tapir config MQTT topic",
	Long: `Send TAPIR-POP global config in TapirMsg form to the tapir config MQTT topic.
	The -F option is required and specifies the file containing the global config in YAML format.
	If -R is specified, will send a retained message, otherwise will send a normal message.
	If -C is specified, will clear the retained config message, otherwise will send the new config.`,
	Run: func(cmd *cobra.Command, args []string) {

		var statusch = make(chan tapir.ComponentStatusUpdate, 10)

		// If any status updates arrive, print them out
		go func() {
			for {
				select {
				case status := <-statusch:
					fmt.Printf("Status update: %+v\n", status)
				}
			}
		}()

		meng, err := tapir.NewMqttEngine("config", mqttclientid, tapir.TapirPub, statusch, log.Default()) // pub, no sub
		if err != nil {
			fmt.Printf("Error from NewMqttEngine: %v\n", err)
			os.Exit(1)
		}

		if cfgfile == "" {
			fmt.Println("Error: Configuration file not specified")
			os.Exit(1)
		}

		cfgfile = filepath.Clean(cfgfile)
		cfgData, err := os.ReadFile(cfgfile)
		if err != nil {
			fmt.Printf("Error reading configuration file %s: %v\n", cfgfile, err)
			os.Exit(1)
		}

		var cf ConfigFoo
		err = yaml.Unmarshal(cfgData, &cf)
		if err != nil {
			fmt.Printf("Error unmarshalling YAML data from file %s: %v\n", cfgfile, err)
			os.Exit(1)
		}

		fmt.Printf("Global configuration loaded from %s\n", cfgfile)
		pretty, err := yaml.Marshal(cf.GlobalConfig)
		if err != nil {
			fmt.Printf("Error marshalling YAML data: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Global configuration:\n%s\n", string(pretty))

		mqtttopic = viper.GetString("tapir.config.topic")
		if mqtttopic == "" {
			fmt.Println("Error: tapir.config.topic not specified in config")
			os.Exit(1)
		}
		fmt.Printf("Using DNS TAPIR config MQTT topic: %s\n", mqtttopic)
		signkey, err := tapir.FetchMqttSigningKey(mqtttopic, viper.GetString("tapir.config.signingkey"))
		if err != nil {
			fmt.Printf("Error fetching MQTT signing key: %v", err)
			os.Exit(1)
		}
		// meng.AddTopic(mqtttopic, signkey, nil)
		meng.PubSubToTopic(mqtttopic, signkey, nil, nil)

		cmnder, outbox, _, err := meng.StartEngine()
		if err != nil {
			fmt.Printf("Error from StartEngine(): %v\n", err)
			os.Exit(1)
		}

		SetupInterruptHandler(cmnder)

		srcname := viper.GetString("tapir.config.srcname")
		if srcname == "" {
			fmt.Println("Error: tapir.config.srcname not specified in config")
			os.Exit(1)
		}

		var tmsg = tapir.TapirMsg{
			SrcName:      srcname,
			Creator:      "tapir-cli",
			MsgType:      "global-config",
			GlobalConfig: cf.GlobalConfig,
			TimeStamp:    time.Now(),
		}
		if mqttconfigclear {
			tmsg.Msg = ""
			outbox <- tapir.MqttPkg{
				Type:   "text",
				Topic:  mqtttopic,
				Retain: true,
				Msg:    "",
			}
		} else {
			outbox <- tapir.MqttPkg{
				Type:   "data",
				Topic:  mqtttopic,
				Retain: mqttretain,
				Data:   tmsg,
			}
		}

		fmt.Println("[Waiting 1000 ms to ensure message has been sent]")
		// Here we need to hang around for a while to ensure that the message has time to be sent.
		time.Sleep(1000 * time.Millisecond)
		fmt.Printf("Hopefully the config message has been sent.\n")
	},
}

var mqttTapirObservationsCmd = &cobra.Command{
	Use:   "observations",
	Short: "Send observations in TapirMsg form to the tapir intel MQTT topic (debug tool)",
	Long: `Will query for operation (add|del|show|send|set-ttl|list-tags|quit), domain name and tags.
Will end the loop on the operation (or domain name) "QUIT"`,
	Run: func(cmd *cobra.Command, args []string) {

		var statusch = make(chan tapir.ComponentStatusUpdate, 10)

		// If any status updates arrive, print them out
		go func() {
			for {
				select {
				case status := <-statusch:
					fmt.Printf("Status update: %+v\n", status)
				}
			}
		}()

		meng, err := tapir.NewMqttEngine("observations", mqttclientid, tapir.TapirPub, statusch, log.Default()) // pub, no sub
		if err != nil {
			fmt.Printf("Error from NewMqttEngine: %v\n", err)
			os.Exit(1)
		}

		mqtttopic = viper.GetString("tapir.observations.topic")
		if mqtttopic == "" {
			fmt.Println("Error: tapir.observations.topic not specified in config")
			os.Exit(1)
		}
		fmt.Printf("Using DNS TAPIR observation MQTT topic: %s\n", mqtttopic)

		signkey, err := tapir.FetchMqttSigningKey(mqtttopic, viper.GetString("tapir.observations.signingkey"))
		if err != nil {
			log.Fatalf("Error fetching MQTT signing key: %v", err)
		}
		// meng.AddTopic(mqtttopic, signkey, nil)
		meng.PubSubToTopic(mqtttopic, signkey, nil, nil)

		cmnder, outbox, _, err := meng.StartEngine()
		if err != nil {
			log.Fatalf("Error from StartEngine(): %v", err)
		}

		count := 0

		SetupInterruptHandler(cmnder)

		srcname := viper.GetString("tapir.observations.srcname")
		if srcname == "" {
			fmt.Println("Error: tapir.observations.srcname not specified in config")
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
			op = tapir.TtyRadioButtonQ("Operation", "add", ops)
			switch op {
			case "quit":
				fmt.Println("QUIT cmd recieved.")
				break cmdloop

			case "set-ttl":
				ttl = tapir.TtyIntQuestion("TTL (in seconds)", 60, false)
				// fmt.Printf("TTL: got: %d\n", tmp)
				// ttl = time.Duration(tmp) * time.Second
				// fmt.Printf("TTL: got: %d ttl: %v\n", tmp, ttl)
			case "add", "del":
				names = tapir.TtyQuestion("Domain names", names, false)
				snames = strings.Fields(names)
				if len(snames) > 0 && strings.ToUpper(snames[0]) == "QUIT" {
					break cmdloop
				}

				if op == "add" {
				retry:
					for {
						tags = tapir.TtyQuestion("Tags", tags, false)
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
				if tapir.GlobalCF.Verbose {
					fmt.Printf("Sending TAPIR-POP observation message to topic %s\n", mqtttopic)
				}
				outbox <- tapir.MqttPkg{
					Type:   "data",
					Topic:  mqtttopic,
					Retain: false,
					Data:   tmsg,
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

var mqttTapirStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Send status updates to the tapir intel MQTT topic (debug tool)",
	Long: `Will query for operation (add|del|show|send|set-ttl|list-tags|quit), component name and status.
Will end the loop on the operation (or component name) "QUIT"`,
	Run: func(cmd *cobra.Command, args []string) {

		var statusch = make(chan tapir.ComponentStatusUpdate, 10)

		// If any status updates arrive, print them out
		go func() {
			for {
				select {
				case status := <-statusch:
					fmt.Printf("Status update: %+v\n", status)
				}
			}
		}()

		meng, err := tapir.NewMqttEngine("status", mqttclientid, tapir.TapirPub, statusch, log.Default()) // pub, no sub
		if err != nil {
			fmt.Printf("Error from NewMqttEngine: %v\n", err)
			os.Exit(1)
		}

		mqtttopic = viper.GetString("tapir.status.topic")
		if mqtttopic == "" {
			fmt.Println("Error: tapir.status.topic not specified in config")
			os.Exit(1)
		}
		fmt.Printf("Using DNS TAPIR status MQTT topic: %s\n", mqtttopic)

		signkey, err := tapir.FetchMqttSigningKey(mqtttopic, viper.GetString("tapir.status.signingkey"))
		if err != nil {
			log.Fatalf("Error fetching MQTT signing key: %v", err)
		}
		// meng.AddTopic(mqtttopic, signkey, nil)
		meng.PubSubToTopic(mqtttopic, signkey, nil, nil)

		cmnder, outbox, _, err := meng.StartEngine()
		if err != nil {
			log.Fatalf("Error from StartEngine(): %v", err)
		}

		count := 0

		SetupInterruptHandler(cmnder)

		//		srcname := viper.GetString("tapir.status.srcname")
		//		if srcname == "" {
		//			fmt.Println("Error: tapir.status.srcname not specified in config")
		//			os.Exit(1)
		//		}

		var op, cname, status string
		var tmsg = tapir.TapirMsg{
			SrcName:   "status",
			Creator:   "tapir-cli",
			MsgType:   "status",
			TimeStamp: time.Now(),
		}

		var ops = []string{"add", "del", "show", "send", "set-ttl", "list-tags", "quit"}
		fmt.Printf("Defined operations are: %v\n", ops)

		tfs := tapir.TapirFunctionStatus{
			Function:   "tapir-pop",
			FunctionID: mqttfid,
			ComponentStatus: map[string]tapir.TapirComponentStatus{
				"downstream-notify": {
					Component: "downstream-notify",
					Status:    "failure",
					ErrorMsg:  "Downstream notify is boiling over",
				},
			},
		}

		known_components := []string{"downstream-notify", "main-boot", "rpz-update", "mqtt-msg", "config", "rpz-update"}

	cmdloop:
		for {
			count++
			op = tapir.TtyRadioButtonQ("Operation", "add", ops)
			switch op {
			case "quit":
				fmt.Println("QUIT cmd recieved.")
				break cmdloop

			case "add", "del":
				cname = tapir.TtyQuestion("Component name", cname, false)
				if len(cname) > 0 && strings.ToUpper(cname) == "QUIT" {
					break cmdloop
				}
				if op == "del" {
					delete(tfs.ComponentStatus, cname)
					continue
				}

				for {
					status = tapir.TtyQuestion("Status", status, false)
					switch status {
					case "ok", "fail", "warn":
						break
					default:
						fmt.Printf("Error: unknown status: %s\n", status)
						status = "fail"
						continue
					}
					break
				}

				_, exist := tfs.ComponentStatus[cname]
				if !exist {
					tfs.ComponentStatus[cname] = tapir.TapirComponentStatus{
						Component: cname,
						Status:    "ok",
						ErrorMsg:  "",
					}
				}
				comp := tfs.ComponentStatus[cname]
				comp.Status = status
				if status == "fail" {
					comp.LastFail = time.Now()
					comp.NumFails++
					comp.ErrorMsg = tapir.TtyQuestion("Error message", "", false)
				} else {
					comp.LastSuccess = time.Now()
					comp.ErrorMsg = ""
					comp.Msg = tapir.TtyQuestion("Message", "", false)
				}
				tfs.ComponentStatus[cname] = comp

			case "show":
				var out = []string{"Component|Status|ErrorMsg|Msg|NumFailures|LastFailure|LastSuccess"}
				for cname, comp := range tfs.ComponentStatus {
					out = append(out, fmt.Sprintf("%s|%s|%s|%s|%d|%s|%s", cname, comp.Status, comp.ErrorMsg, comp.Msg, comp.NumFails,
						comp.LastFail.Format(tapir.TimeLayout), comp.LastSuccess.Format(tapir.TimeLayout)))
				}
				fmt.Println(columnize.SimpleFormat(out))

			case "list-comp":
				fmt.Printf("%v\n", known_components)

			case "send":
				if tapir.GlobalCF.Verbose {
					fmt.Printf("Sending TAPIR-POP status message to topic %s\n", mqtttopic)
				}
				tmsg.TapirFunctionStatus = tfs
				outbox <- tapir.MqttPkg{
					Type:   "data",
					Topic:  mqtttopic,
					Retain: false,
					Data:   tmsg,
				}

				tmsg = tapir.TapirMsg{
					Creator:   "tapir-cli",
					MsgType:   "status",
					TimeStamp: time.Now(),
				}

			}
		}
		respch := make(chan tapir.MqttEngineResponse, 2)
		meng.CmdChan <- tapir.MqttEngineCmd{Cmd: "stop", Resp: respch}
		r := <-respch
		fmt.Printf("Response from MQTT Engine: %v\n", r)
	},
}

var mqttTapirBootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "MQTT Bootstrap commands",
}

var mqttTapirBootstrapStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Send send greylist-status request to MQTT Bootstrap Server",
	Run: func(cmd *cobra.Command, args []string) {
		srcs, err := ParseSources()
		if err != nil {
			log.Fatalf("Error parsing sources: %v", err)
		}

		var src *SourceConf
		for k, v := range srcs {
			// fmt.Printf("Src: %s, Name: %s, Type: %s, Bootstrap: %v\n", k, v.Name, v.Type, v.Bootstrap)
			if v.Name == mqttgreylist && v.Source == "mqtt" && v.Type == "greylist" {
				src = &v

				PrintBootstrapMqttStatus(k, src)
			}
		}

		if src == nil {
			fmt.Printf("Error: greylist source \"%s\" not found in sources", mqttgreylist)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(mqttCmd)
	mqttCmd.AddCommand(mqttEngineCmd, mqttTapirCmd)
	mqttTapirCmd.AddCommand(mqttTapirObservationsCmd, mqttTapirConfigCmd, mqttTapirStatusCmd, mqttTapirBootstrapCmd)
	mqttTapirBootstrapCmd.AddCommand(mqttTapirBootstrapStatusCmd)

	mqttCmd.PersistentFlags().StringVarP(&mqtttopic, "topic", "t", "", "MQTT topic, default from tapir-cli config")

	mqttclientid = "tapir-cli-" + uuid.New().String()
	mqttCmd.PersistentFlags().StringVarP(&mqttclientid, "clientid", "", mqttclientid, "MQTT client id, default is a random string")
	mqttEngineCmd.Flags().BoolVarP(&mqttpub, "pub", "", false, "Enable pub support")
	mqttEngineCmd.Flags().BoolVarP(&mqttsub, "sub", "", false, "Enable sub support")
	mqttTapirConfigCmd.Flags().BoolVarP(&mqttretain, "retain", "R", false, "Publish a retained message")
	mqttTapirConfigCmd.Flags().BoolVarP(&mqttconfigclear, "clear", "C", false, "Clear retained config message")
	mqttTapirConfigCmd.Flags().StringVarP(&cfgfile, "cfgfile", "F", "", "Name of file containing global config to send")
	mqttTapirBootstrapCmd.PersistentFlags().StringVarP(&mqttgreylist, "greylist", "G", "dns-tapir", "Greylist to inquire about")

	mqttTapirStatusCmd.Flags().StringVarP(&mqttfid, "functionid", "F", "tapir-cli debug tool", "Function ID to send status for")
}

func PrintBootstrapMqttStatus(name string, src *SourceConf) error {
	if len(src.Bootstrap) == 0 {
		if len(src.Bootstrap) == 0 {
			fmt.Printf("Note: greylist source %s (name \"%s\") has no bootstrap servers\n", name, src.Name)
			return fmt.Errorf("no bootstrap servers")
		}
	}

	// Initialize the API client
	api := &tapir.ApiClient{
		BaseUrl:    fmt.Sprintf(src.BootstrapUrl, src.Bootstrap[0]), // Must specify a valid BaseUrl
		ApiKey:     src.BootstrapKey,
		AuthMethod: "X-API-Key",
	}

	cd := viper.GetString("certs.certdir")
	if cd == "" {
		log.Fatalf("Error: missing config key: certs.certdir")
	}
	// cert := cd + "/" + certname
	cert := cd + "/" + "tapir-pop"
	tlsConfig, err := tapir.NewClientConfig(viper.GetString("certs.cacertfile"), cert+".key", cert+".crt")
	if err != nil {
		log.Fatalf("BootstrapMqttSource: Error: Could not set up TLS: %v", err)
	}
	// XXX: Need to verify that the server cert is valid for the bootstrap server
	tlsConfig.InsecureSkipVerify = true
	err = api.SetupTLS(tlsConfig)
	if err != nil {
		return fmt.Errorf("error setting up TLS for the API client: %v", err)
	}

	// out := []string{"Server|Uptime|Src|Name|MQTT Topic|Msgs|LastMsg"}
	out := []string{"Server|Uptime|Src|Name|MQTT Topic|Pub Msgs|LastPub|Sub Msgs|LastSub"}

	// Iterate over the bootstrap servers
	for _, server := range src.Bootstrap {
		api.BaseUrl = fmt.Sprintf(src.BootstrapUrl, server)

		// Send an API ping command
		pr, err := api.SendPing(0, false)
		if err != nil {
			fmt.Printf("Ping to MQTT bootstrap server %s failed: %v\n", server, err)
			continue
		}

		uptime := time.Since(pr.BootTime).Round(time.Second)
		// fmt.Printf("MQTT bootstrap server %s uptime: %v. It has processed %d MQTT messages", server, uptime, 17)

		status, buf, err := api.RequestNG(http.MethodPost, "/bootstrap", tapir.BootstrapPost{
			Command:  "greylist-status",
			ListName: src.Name,
			Encoding: "json", // XXX: This is our default, but we'll test other encodings later
		}, true)
		if err != nil {
			fmt.Printf("Error from RequestNG: %v\n", err)
			continue
		}

		if status != http.StatusOK {
			fmt.Printf("Bootstrap server %s responded with error: %s (instead of greylist status)\n", server, http.StatusText(status))
			continue
		}

		var br tapir.BootstrapResponse
		err = json.Unmarshal(buf, &br)
		if err != nil {
			fmt.Printf("Error decoding greylist-status response from %s: %v. Giving up.\n", server, err)
			continue
		}
		if br.Error {
			fmt.Printf("Bootstrap server %s responded with error: %s (instead of greylist status)\n", server, br.ErrorMsg)
		}
		if tapir.GlobalCF.Verbose && len(br.Msg) != 0 {
			fmt.Printf("MQTT Bootstrap server %s responded with message: %s\n", server, br.Msg)
		}

		//		for topic, v := range br.MsgCounters {
		//			out = append(out, fmt.Sprintf("%s|%v|%s|%s|%s|%d|%s", server, uptime, name, src.Name, topic, v, br.MsgTimeStamps[topic].Format(time.RFC3339)))
		//		}

		for topic, topicdata := range br.TopicData {
			out = append(out, fmt.Sprintf("%s|%v|%s|%s|%s|%d|%s|%d|%s", server, uptime, name, src.Name, topic, topicdata.PubMsgs, topicdata.LatestPub.Format(time.RFC3339), topicdata.SubMsgs, topicdata.LatestSub.Format(time.RFC3339)))
		}
	}

	fmt.Println(columnize.SimpleFormat(out))

	return nil
}

type SrcFoo struct {
	Src struct {
		Style string `yaml:"style"`
	} `yaml:"src"`
	Sources map[string]SourceConf `yaml:"sources"`
}

type SourceConf struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	Type         string `yaml:"type"`
	Topic        string `yaml:"topic"`
	Source       string `yaml:"source"`
	SrcFormat    string `yaml:"src_format"`
	Format       string `yaml:"format"`
	Datasource   string `yaml:"datasource"`
	Bootstrap    []string
	BootstrapUrl string
	BootstrapKey string
}

func ParseSources() (map[string]SourceConf, error) {
	var srcfoo SrcFoo
	configFile := filepath.Clean(tapir.PopSourcesCfgFile)
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	err = yaml.Unmarshal(data, &srcfoo)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling YAML data: %v", err)
	}

	srcs := srcfoo.Sources
	// fmt.Printf("*** ParseSourcesNG: there are %d defined sources in config\n", len(srcs))
	return srcs, nil
}

func SetupSubPrinter(inbox chan tapir.MqttPkg) {
	go func() {
		var pkg tapir.MqttPkg
		for {
			select {
			case pkg = <-inbox:
				var out []string
				fmt.Printf("Received TAPIR MQTT Message of type: %s\n", pkg.Data.MsgType)
				for _, a := range pkg.Data.Added {
					out = append(out, fmt.Sprintf("ADD: %s|%032b", a.Name, a.TagMask))
				}
				for _, a := range pkg.Data.Removed {
					out = append(out, fmt.Sprintf("DEL: %s", a.Name))
				}
				fmt.Println(columnize.SimpleFormat(out))
				pretty, err := yaml.Marshal(pkg.Data)
				if err != nil {
					fmt.Printf("Error marshalling YAML data: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("Received TAPIR MQTT Message:\n%s\n", string(pretty))
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
