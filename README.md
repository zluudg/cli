# tapir-cli

CLI utility primarily to interact with other DNS TAPIR components.
For this the config details needed to connect to eg. TAPIR-POP are
located in the config files in the /etc/dnstapir/ directory.

For some uses, though, `tapir-cli` is used in "standalone" mode. One
example of this is the creation anad manipulation of DAWG files
containing large lists of domain names in a compact format. To run
`tapir-cli` in standalone mode, without the need for any config files,
use the command flag `--standalone`.

`tapir-cli` has a large number of commands and subcommands. The entire
set of commands is structured as a tree with the root in the
`tapi-cli` command.  All commands, regardless of where in the tree of
commands they are located, have online help via the flag `-h`. I.e. to
get help on the `tapir-cli pop ping` command, run:

```
tapir-cli pop ping -h
Send an API ping request to TAPIR-POP and present the response

Usage:
  tapir-cli pop ping [flags]

Flags:
  -c, --count int   #pings to send
  -h, --help        help for ping
  -n, --newapi      use new api client

Global Flags:
      --config string   config file (default is /etc/dnstapir/tapir-pop.yaml)
  -d, --debug           Debugging output
  -H, --headers         Show column headers
      --tls             Use a TLS connection to TAPIR-POP (default true)
  -v, --verbose         Verbose mode
```

The flag `-h` also lists all subcommands underneath the command in question.

The `tapir-cli` command has a number of subcommands, each of which is a command group. The command groups are:

```
tapir-cli -h                
CLI  utility used to interact with TAPIR-POP, i.e. the TAPIR Policy Processor

Usage:
  tapir-cli [command]

Available Commands:
  api         request a TAPIR-POP api summary
  bump        Instruct TAPIR-POP to bump the SOA serial of the RPZ zone
  completion  Generate the autocompletion script for the specified shell
  dawg        Generate or interact with data stored in a DAWG file; only useable via sub-commands
  debug       A brief description of your command
  help        Help about any command
  pop         Prefix command, only usable via sub-commands
  rpz         Instruct TAPIR-POP to modify the RPZ zone; must use sub-command

Flags:
      --config string   config file (default is /etc/dnstapir/tapir-pop.yaml)
  -d, --debug           Debugging output
  -H, --headers         Show column headers
  -h, --help            help for tapir-cli
      --tls             Use a TLS connection to TAPIR-POP (default true)
  -v, --verbose         Verbose mode

Use "tapir-cli [command] --help" for more information about a command.
```

Many of the commands are only there as debugging tools. They are not intended for use in production. 
