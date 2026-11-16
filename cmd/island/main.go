package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/islandweave/islandweave/core/identity"
)

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".islandweave"
	}
	return filepath.Join(home, ".islandweave")
}

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(dataDir string) (*client, error) {
	addrBytes, err := os.ReadFile(filepath.Join(dataDir, "control.addr"))
	if err != nil {
		return nil, fmt.Errorf("islandd does not appear to be running (missing %s/control.addr): %w", dataDir, err)
	}
	tokBytes, err := os.ReadFile(filepath.Join(dataDir, "control.token"))
	if err != nil {
		return nil, fmt.Errorf("missing control token: %w", err)
	}
	return &client{
		baseURL: "http://" + strings.TrimSpace(string(addrBytes)),
		token:   strings.TrimSpace(string(tokBytes)),
		http:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *client) do(method, path string, body any) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.baseURL+path, r)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return out, resp.StatusCode, err
}

func main() {
	global := flag.NewFlagSet("island", flag.ExitOnError)
	dataDir := global.String("data-dir", defaultDataDir(), "IslandWeave data directory")
	_ = global.Parse(os.Args[1:])

	args := global.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		name := fs.String("name", "", "Node display name")
		_ = fs.Parse(rest)
		id, created, err := identity.LoadOrCreateNodeIdentity(*dataDir, *name)
		fatalIf(err)
		if created {
			fmt.Printf("Initialized new IslandWeave node: %s (%s)\n", id.ID, id.Name)
		} else {
			fmt.Printf("Existing IslandWeave node: %s (%s)\n", id.ID, id.Name)
		}
	case "start":
		fs := flag.NewFlagSet("start", flag.ExitOnError)
		listen := fs.String("listen", "0.0.0.0:47472", "P2P listen address")
		control := fs.String("control", "127.0.0.1:47471", "Control API address")
		name := fs.String("name", "", "Node name")
		_ = fs.Parse(rest)

		bin, err := findDaemonBinary()
		fatalIf(err)
		cArgs := []string{"--data-dir", *dataDir, "--listen", *listen, "--control", *control}
		if *name != "" {
			cArgs = append(cArgs, "--name", *name)
		}
		logPath := filepath.Join(*dataDir, "islandd.log")
		_ = os.MkdirAll(*dataDir, 0o700)
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		fatalIf(err)
		proc := exec.Command(bin, cArgs...)
		proc.Stdout = logFile
		proc.Stderr = logFile
		fatalIf(proc.Start())
		_ = logFile.Close()
		fmt.Printf("Started islandd (PID %d), logs at %s\n", proc.Process.Pid, logPath)
	case "stop":
		pidBytes, err := os.ReadFile(filepath.Join(*dataDir, "islandd.pid"))
		fatalIf(err)
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
		fatalIf(err)
		p, err := os.FindProcess(pid)
		fatalIf(err)
		fatalIf(p.Signal(syscall.SIGTERM))
		fmt.Printf("Sent SIGTERM to islandd (PID %d)\n", pid)
	case "status":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/status", nil)
		fatalIf(err)
		printResp(out, code)
	case "create":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island create <name> [--visibility public|private|invite-only]")
			os.Exit(1)
		}
		name := rest[0]
		fs := flag.NewFlagSet("create", flag.ExitOnError)
		vis := fs.String("visibility", "invite-only", "Island visibility")
		_ = fs.Parse(rest[1:])
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/islands", map[string]string{
			"name":       name,
			"visibility": *vis,
		})
		fatalIf(err)
		printResp(out, code)
	case "islands":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/islands", nil)
		fatalIf(err)
		printResp(out, code)
	case "invite":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island invite <island-id>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/islands/invite", map[string]any{
			"island_id": rest[0],
		})
		fatalIf(err)
		printResp(out, code)
	case "join":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island join <invite-token>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/islands/join", map[string]string{
			"token": rest[0],
		})
		fatalIf(err)
		printResp(out, code)
	case "leave":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island leave <island-id>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/islands/leave", map[string]string{
			"island_id": rest[0],
		})
		fatalIf(err)
		printResp(out, code)
	case "discover":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/discover/merged", nil)
		fatalIf(err)
		printResp(out, code)
	case "routes":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/routes", nil)
		fatalIf(err)
		printResp(out, code)
	case "sync":
		c, err := newClient(*dataDir)
		fatalIf(err)
		var peerID string
		if len(rest) > 0 {
			peerID = rest[0]
		}
		out, code, err := c.do(http.MethodPost, "/v1/sync", map[string]string{"peer_id": peerID})
		fatalIf(err)
		printResp(out, code)
	case "peers":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/peers", nil)
		fatalIf(err)
		printResp(out, code)
	case "connect":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island connect <host:port>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/peers/connect", map[string]string{
			"addr": rest[0],
		})
		fatalIf(err)
		printResp(out, code)
	case "trust":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island trust <node-id>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/peers/trust", map[string]string{
			"node_id": rest[0],
		})
		fatalIf(err)
		printResp(out, code)
	case "revoke":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island revoke <node-id>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/peers/revoke", map[string]string{
			"node_id": rest[0],
		})
		fatalIf(err)
		printResp(out, code)
	case "beacon":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island beacon <on|off|status> [--mode public|restricted] [--island <id>]")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		switch rest[0] {
		case "status":
			out, code, err := c.do(http.MethodGet, "/v1/beacon", nil)
			fatalIf(err)
			printResp(out, code)
		case "off":
			out, code, err := c.do(http.MethodPost, "/v1/beacon", map[string]string{"mode": "silent"})
			fatalIf(err)
			printResp(out, code)
		case "on":
			fs := flag.NewFlagSet("beacon-on", flag.ExitOnError)
			mode := fs.String("mode", "public", "Beacon mode (public|restricted)")
			islandID := fs.String("island", "", "Island ID to advertise")
			_ = fs.Parse(rest[1:])
			out, code, err := c.do(http.MethodPost, "/v1/beacon", map[string]string{
				"mode":      *mode,
				"island_id": *islandID,
			})
			fatalIf(err)
			printResp(out, code)
		default:
			fmt.Fprintf(os.Stderr, "unknown beacon subcommand: %s\n", rest[0])
			os.Exit(1)
		}
	case "send-msg":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "usage: island send-msg <node-id> <message>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/messages/send", map[string]string{
			"to":   rest[0],
			"body": strings.Join(rest[1:], " "),
		})
		fatalIf(err)
		printResp(out, code)
	case "messages":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/messages", nil)
		fatalIf(err)
		printResp(out, code)
	case "host":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island host <dir> [--name <name>] [--island <id>]")
			os.Exit(1)
		}
		dir := rest[0]
		fs := flag.NewFlagSet("host", flag.ExitOnError)
		name := fs.String("name", "", "Website name")
		islandID := fs.String("island", "", "Island ID")
		_ = fs.Parse(rest[1:])
		absDir, err := filepath.Abs(dir)
		fatalIf(err)
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/services/host", map[string]string{
			"dir":       absDir,
			"name":      *name,
			"island_id": *islandID,
		})
		fatalIf(err)
		printResp(out, code)
	case "services":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/services", nil)
		fatalIf(err)
		printResp(out, code)
	case "fetch":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island fetch <service-name-or-id>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/services/fetch", map[string]string{
			"name_or_id": rest[0],
		})
		fatalIf(err)
		printResp(out, code)
	case "send":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island send <file> --to <node-id>")
			os.Exit(1)
		}
		filePath := rest[0]
		fs := flag.NewFlagSet("send", flag.ExitOnError)
		to := fs.String("to", "", "Recipient Node ID")
		_ = fs.Parse(rest[1:])
		if *to == "" {
			fmt.Fprintln(os.Stderr, "--to <node-id> is required")
			os.Exit(1)
		}
		absFile, err := filepath.Abs(filePath)
		fatalIf(err)
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/transfers/send", map[string]string{
			"to":        *to,
			"file_path": absFile,
		})
		fatalIf(err)
		printResp(out, code)
	case "transfers":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/transfers", nil)
		fatalIf(err)
		printResp(out, code)
	case "cancel-transfer":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island cancel-transfer <transfer-id>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/transfers/cancel", map[string]string{
			"transfer_id": rest[0],
		})
		fatalIf(err)
		printResp(out, code)
	case "courier":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island courier <status|queue|send|export|import> [args]")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		switch rest[0] {
		case "status":
			out, code, err := c.do(http.MethodGet, "/v1/courier/status", nil)
			fatalIf(err)
			printResp(out, code)
		case "queue":
			out, code, err := c.do(http.MethodGet, "/v1/courier/queue", nil)
			fatalIf(err)
			printResp(out, code)
		case "send":
			if len(rest) < 3 {
				fmt.Fprintln(os.Stderr, "usage: island courier send <node-id> <message>")
				os.Exit(1)
			}
			out, code, err := c.do(http.MethodPost, "/v1/courier/send", map[string]any{
				"to":   rest[1],
				"body": strings.Join(rest[2:], " "),
			})
			fatalIf(err)
			printResp(out, code)
		case "export":
			if len(rest) < 2 {
				fmt.Fprintln(os.Stderr, "usage: island courier export <file.iwcourier>")
				os.Exit(1)
			}
			abs, err := filepath.Abs(rest[1])
			fatalIf(err)
			out, code, err := c.do(http.MethodPost, "/v1/courier/export", map[string]string{"file_path": abs})
			fatalIf(err)
			printResp(out, code)
		case "import":
			if len(rest) < 2 {
				fmt.Fprintln(os.Stderr, "usage: island courier import <file.iwcourier>")
				os.Exit(1)
			}
			abs, err := filepath.Abs(rest[1])
			fatalIf(err)
			out, code, err := c.do(http.MethodPost, "/v1/courier/import", map[string]string{"file_path": abs})
			fatalIf(err)
			printResp(out, code)
		default:
			fmt.Fprintf(os.Stderr, "unknown courier subcommand: %s\n", rest[0])
			os.Exit(1)
		}
	case "mail":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island mail <send|inbox|outbox|drafts|sent> [args]")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		switch rest[0] {
		case "inbox", "outbox", "drafts", "sent":
			out, code, err := c.do(http.MethodGet, "/v1/mail?folder="+rest[0], nil)
			fatalIf(err)
			printResp(out, code)
		case "send":
			fs := flag.NewFlagSet("mail-send", flag.ExitOnError)
			to := fs.String("to", "", "Recipient Node ID")
			subj := fs.String("subject", "", "Mail subject")
			body := fs.String("body", "", "Mail body")
			_ = fs.Parse(rest[1:])
			if *to == "" || *body == "" {
				fmt.Fprintln(os.Stderr, "usage: island mail send --to <node-id> --subject <subj> --body <text>")
				os.Exit(1)
			}
			out, code, err := c.do(http.MethodPost, "/v1/mail/send", map[string]any{
				"to":      *to,
				"subject": *subj,
				"body":    *body,
			})
			fatalIf(err)
			printResp(out, code)
		default:
			fmt.Fprintf(os.Stderr, "unknown mail subcommand: %s\n", rest[0])
			os.Exit(1)
		}
	case "publish":
		fs := flag.NewFlagSet("publish", flag.ExitOnError)
		channel := fs.String("channel", "community-news", "Bulletin channel")
		title := fs.String("title", "", "Bulletin title")
		summary := fs.String("summary", "", "Short summary")
		body := fs.String("body", "", "Bulletin body")
		_ = fs.Parse(rest)
		if *title == "" {
			fmt.Fprintln(os.Stderr, "usage: island publish --channel <ch> --title <title> --body <text>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/bulletins/publish", map[string]any{
			"channel": *channel,
			"title":   *title,
			"summary": *summary,
			"body":    *body,
		})
		fatalIf(err)
		printResp(out, code)
	case "bulletins":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/bulletins", nil)
		fatalIf(err)
		printResp(out, code)
	case "server":
		if len(rest) < 1 || (rest[0] != "enable" && rest[0] != "disable" && rest[0] != "status") {
			fmt.Fprintln(os.Stderr, "usage: island server <enable|disable|status>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		if rest[0] == "status" {
			out, code, err := c.do(http.MethodGet, "/v1/gateway", nil)
			fatalIf(err)
			printResp(out, code)
			return
		}
		out, code, err := c.do(http.MethodPost, "/v1/server/enable", map[string]bool{"relay_enabled": rest[0] == "enable"})
		fatalIf(err)
		printResp(out, code)
	case "gateway":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/gateway", nil)
		fatalIf(err)
		printResp(out, code)
	case "share":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island share <dir> [--name <alias>] [--island <id>]")
			os.Exit(1)
		}
		absDir, err := filepath.Abs(rest[0])
		fatalIf(err)
		fs := flag.NewFlagSet("share", flag.ExitOnError)
		svcName := fs.String("name", "", "Shared library alias")
		islID := fs.String("island", "", "Optional Island ID")
		_ = fs.Parse(rest[1:])
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodPost, "/v1/shares", map[string]string{
			"dir":       absDir,
			"name":      *svcName,
			"island_id": *islID,
		})
		fatalIf(err)
		printResp(out, code)
	case "security":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/security/status", nil)
		fatalIf(err)
		printResp(out, code)
	case "activity":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/activity", nil)
		fatalIf(err)
		printResp(out, code)
	case "diagnostics":
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/diagnostics", nil)
		fatalIf(err)
		printResp(out, code)
	case "tor":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island tor <status|config> [--policy ...] [--socks ...]")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		switch rest[0] {
		case "status":
			out, code, err := c.do(http.MethodGet, "/v1/tor/status", nil)
			fatalIf(err)
			printResp(out, code)
		case "config":
			fs := flag.NewFlagSet("tor-config", flag.ExitOnError)
			policy := fs.String("policy", "tor-only", "Policy: direct-first|tor-preferred|tor-only")
			socks := fs.String("socks", "127.0.0.1:9050", "SOCKS5 proxy address")
			onion := fs.String("onion", "", "Configured .onion:port address")
			_ = fs.Parse(rest[1:])
			out, code, err := c.do(http.MethodPost, "/v1/tor/config", map[string]string{
				"policy":        *policy,
				"socks_addr":    *socks,
				"onion_service": *onion,
			})
			fatalIf(err)
			printResp(out, code)
		default:
			fmt.Fprintf(os.Stderr, "unknown tor subcommand: %s\n", rest[0])
			os.Exit(1)
		}
	case "federation":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: island federation <list|revoke> [args]")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		switch rest[0] {
		case "list":
			out, code, err := c.do(http.MethodGet, "/v1/federation", nil)
			fatalIf(err)
			printResp(out, code)
		case "revoke":
			if len(rest) < 2 {
				fmt.Fprintln(os.Stderr, "usage: island federation revoke <agreement-id>")
				os.Exit(1)
			}
			out, code, err := c.do(http.MethodPost, "/v1/federation/revoke", map[string]string{"agreement_id": rest[1]})
			fatalIf(err)
			printResp(out, code)
		default:
			fmt.Fprintf(os.Stderr, "unknown federation subcommand: %s\n", rest[0])
			os.Exit(1)
		}
	case "satellite":
		if len(rest) < 1 || (rest[0] != "status" && rest[0] != "passes") {
			fmt.Fprintln(os.Stderr, "usage: island satellite <status|passes>")
			os.Exit(1)
		}
		c, err := newClient(*dataDir)
		fatalIf(err)
		out, code, err := c.do(http.MethodGet, "/v1/satellite/"+rest[0], nil)
		fatalIf(err)
		printResp(out, code)
	case "desktop", "navigator":
		binName := "island-" + cmd
		binPath, err := findSiblingBinary(binName)
		fatalIf(err)
		launchArgs := append([]string{"--data-dir", *dataDir}, rest...)
		proc := exec.Command(binPath, launchArgs...)
		proc.Stdout = os.Stdout
		proc.Stderr = os.Stderr
		proc.Stdin = os.Stdin
		fatalIf(proc.Run())
	default:
		printUsage()
		os.Exit(1)
	}
}

func findSiblingBinary(name string) (string, error) {
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	return exec.LookPath(name)
}

func findDaemonBinary() (string, error) {
	return findSiblingBinary("islandd")
}

func printResp(raw []byte, code int) {
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		pretty, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Println(string(raw))
	}
	if code >= 400 {
		os.Exit(1)
	}
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`IslandWeave CLI (v1.0.0)

Usage:
  island [--data-dir <dir>] <command> [args]

Commands:
  init [--name <name>]                     Initialize node identity
  start / stop / status                    Manage or inspect islandd daemon
  desktop / navigator                      Launch Desktop Control Center or Navigator browser
  create <name>                            Create a new Island
  islands                                  List joined Islands
  invite <island-id>                       Create a signed Island invite token
  join <token>                             Join an Island using an invite token
  leave <island-id>                        Leave an Island
  discover                                 List Beacon + peer-exchange discovered peers
  peers                                    List known and connected peers
  routes                                   Show multi-transport IWP-ROUTE table
  sync [peer-id]                           Trigger IWP-SYNC anti-entropy state sync
  connect <host:port>                      Connect to a peer via IWP-LINK
  trust <node-id>                          Mark a peer as trusted
  revoke <node-id>                         Revoke a peer and close active link
  beacon on|off|status                     Control Beacon Mode
  send-msg <node-id> <message>             Send an encrypted message to a peer
  messages                                 List stored messages
  host <dir> [--name <name>]               Host a static website on IslandWeave
  share <dir> [--name <name>]              Publish a browsable Shared File Library
  services                                 List hosted and cached services
  fetch <service-name-or-id>               Fetch and cache a peer's website locally
  gateway list                             List Gateway & hosted endpoint mappings
  send <file> --to <node-id>               Send a file via IWTP
  transfers                                List active and completed file transfers
  cancel-transfer <transfer-id>            Cancel a file transfer
  courier status|queue|send|export|import  Manage delay-tolerant courier mode
  mail send|inbox|outbox|drafts|sent       Manage encrypted IWMP mail
  publish / bulletins                      Publish or read signed IWBN bulletins
  server enable|disable|status             Manage or inspect VPS relay server mode
  tor status|config                        Inspect or configure Tor SOCKS5 policy
  federation list|revoke                   Manage inter-Island federation agreements
  satellite status|passes                  Inspect IWST satellite pass status
  security / activity / diagnostics        Inspect security posture, logs, and diagnostics`)
}
