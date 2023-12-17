// Copyright 2023 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// ark-serman manages Ark Dedicated Servers via systemd.
package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sort"
	"text/template"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/gorcon/rcon"
	"github.com/maruel/serve-dir/loghttp"
	"github.com/maruel/subcommands"
)

//go:embed rsc/*.html.tmpl rsc/static/*
var rsc embed.FS

var application = &subcommands.DefaultApplication{
	Name:  "ark-serman",
	Title: "Ark Dedicated Server Manager.",
	Commands: []*subcommands.Command{
		cmdInstall,
		cmdRCon,
		cmdWeb,
		subcommands.CmdHelp,
	},
}

type args struct {
	subcommands.CommandRunBase
	quiet bool
}

func (a *args) flags() {
	a.Flags.BoolVar(&a.quiet, "q", false, "don't print log lines")
}

//

var cmdInstall = &subcommands.Command{
	UsageLine: "install <options>",
	ShortDesc: "Installs ark-serman and the Ark servers as a systemd service",
	LongDesc:  "Installs ark-serman and the Ark servers as a systemd service.",
	CommandRun: func() subcommands.CommandRun {
		c := &installRun{}
		c.args.flags()
		c.Flags.StringVar(&c.userPwd, "u", "", "user password (optional)")
		c.Flags.StringVar(&c.adminPwd, "a", "", "rcon (admin) password")
		return c
	},
}

type installRun struct {
	args
	userPwd  string
	adminPwd string
}

func (i *installRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) != 0 {
		fmt.Fprintf(os.Stderr, "%s: Unsupported arguments.\n", a.GetName())
		return 1
	}
	//ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	//defer cancel()
	fmt.Fprintf(os.Stderr, "TODO(maruel): Implement me\n")
	return 1
}

//

var cmdRCon = &subcommands.Command{
	UsageLine: "rcon <options> <commands>",
	ShortDesc: "Connects to an Ark server via RCon (admin) port",
	LongDesc:  "Connects to an Ark server via RCon (admin) port.",
	CommandRun: func() subcommands.CommandRun {
		c := &rconRun{}
		c.args.flags()
		c.Flags.StringVar(&c.host, "p", "", "rcon host:port")
		c.Flags.StringVar(&c.adminPwd, "a", "", "rcon (admin) password")
		return c
	},
}

type rconRun struct {
	args
	host     string
	adminPwd string
}

func (r *rconRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "%s: At least one admin command required.\n", a.GetName())
		return 1
	}
	// Doesn't support context at the moment.
	conn, err := rcon.Dial(r.host, r.adminPwd)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	for _, a := range args {
		fmt.Printf("Running: %s\n", a)
		resp, err := conn.Execute(a)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  Got: %s\n", resp)
	}
	return 0
}

//

var cmdWeb = &subcommands.Command{
	UsageLine: "web <options>",
	ShortDesc: "Runs the web server",
	LongDesc:  "Runs the web server to manage the Ark servers.\nSee rcon commands at https://ark.fandom.com/wiki/Console_commands\n",
	CommandRun: func() subcommands.CommandRun {
		c := &webRun{}
		c.args.flags()
		c.Flags.StringVar(&c.bind, "p", ":8070", "bind address and port")
		c.Flags.StringVar(&c.adminPwd, "pwd", "", "rcon (admin) password")
		return c
	},
}

var pageTmpl = template.Must(template.ParseFS(rsc, "rsc/root.html.tmpl"))

func replyError(w http.ResponseWriter, s string) {
	w.Header().Add("Content-Type", "text/plain")
	w.WriteHeader(500)
	io.WriteString(w, s)
}

type unitStatus struct {
	dbus.UnitStatus
	Props       map[string]interface{}
	Running     bool
	DisplayName string
	CPU         float64
	Memory      float64
}

func round(val float64, precision int) float64 {
	return math.Round(val*(math.Pow10(precision))) / math.Pow10(precision)
}

func getUnitStates(ctx context.Context) ([]unitStatus, error) {
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	unitFiles, err := conn.ListUnitFilesByPatternsContext(ctx, nil, []string{"ark-*"})
	if err != nil {
		return nil, err
	}
	unitNames := make([]string, 0, len(unitFiles))
	for _, v := range unitFiles {
		b := path.Base(v.Path)
		if b == "ark-serman.service" {
			continue
		}
		unitNames = append(unitNames, b)
	}
	unitStates, err := conn.ListUnitsByNamesContext(ctx, unitNames)
	if err != nil {
		return nil, err
	}
	sort.Slice(unitStates, func(i, j int) bool {
		return unitStates[i].Name < unitStates[j].Name
	})

	out := make([]unitStatus, len(unitStates))
	for i, s := range unitStates {
		out[i].UnitStatus = s
		out[i].DisplayName = s.Name[4 : len(s.Name)-8]
		out[i].Running = s.ActiveState == "active" || s.ActiveState == "activating" || s.ActiveState == "deactivating"
		if out[i].Running {
			// TODO(maruel): Query less, query in parallel.
			p, err := conn.GetAllPropertiesContext(ctx, s.Name)
			if err != nil {
				return nil, err
			}
			out[i].Props = p
			c := p["CPUUsageNSec"].(uint64)
			out[i].CPU = round(float64(c)*0.000000001, 1)
			m := p["MemoryCurrent"].(uint64)
			out[i].Memory = round(float64(m)*0.000001, 1)
		}
		// TODO(maruel): List save games.
		//path := "~/.local/share/Steam/steamapps/common/ARK Survival Evolved Dedicated Server/ShooterGame/Saved/SavedArks"
	}
	return out, nil
}

func serveRoot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, err := getUnitStates(ctx)
	if err != nil {
		replyError(w, err.Error())
		return
	}
	w.Header().Add("Content-Type", "text/html")
	data := map[string]any{
		"Servers": u,
	}
	if err := pageTmpl.Execute(w, data); err != nil {
		log.Fatal(err)
	}
}

func rpcStart(w http.ResponseWriter, r *http.Request) {
	unitName := path.Base(r.URL.Path)
	ctx := r.Context()
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		replyError(w, err.Error())
		return
	}
	defer conn.Close()
	if _, err = conn.StartUnitContext(ctx, unitName, "replace", nil); err != nil {
		replyError(w, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func rpcStop(w http.ResponseWriter, r *http.Request) {
	unitName := path.Base(r.URL.Path)
	ctx := r.Context()
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		replyError(w, err.Error())
		return
	}
	defer conn.Close()
	if _, err = conn.StopUnitContext(ctx, unitName, "replace", nil); err != nil {
		replyError(w, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

type webRun struct {
	args
	bind     string
	adminPwd string
}

func (w *webRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) != 0 {
		fmt.Fprintf(os.Stderr, "%s: Unsupported arguments.\n", a.GetName())
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	mux := &http.ServeMux{}
	mux.Handle("/rpc/start/", http.HandlerFunc(rpcStart))
	mux.Handle("/rpc/stop/", http.HandlerFunc(rpcStop))
	static, err := fs.Sub(rsc, "rsc/static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.Handle("/favicon.ico", http.RedirectHandler("/static/ark.png", http.StatusSeeOther))
	mux.Handle("/", http.HandlerFunc(serveRoot))
	var h http.Handler = mux
	if !w.quiet {
		h = &loghttp.Handler{Handler: mux}
	}
	s := &http.Server{
		Addr:           w.bind,
		Handler:        h,
		ReadTimeout:    10. * time.Second,
		WriteTimeout:   60 * time.Second,
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
		BaseContext:    func(net.Listener) context.Context { return ctx },
	}
	log.Printf("Serving on %s", w.bind)
	log.Fatal(s.ListenAndServe())
	return 0
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	os.Exit(subcommands.Run(application, nil))
}
