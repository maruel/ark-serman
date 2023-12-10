// Copyright 2023 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// ark-serman manages Ark Dedicated Servers via systemd.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"sort"
	"text/template"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/gorcon/rcon"
	"github.com/maruel/serve-dir/loghttp"
)

const page = `<!DOCTYPE html>
<meta name="viewport" content="width=device-width, initial-scale=1" />
<style>
thead {
	background-color: lightgray;
}
tr :nth-child(4), tr :nth-child(5) {
  text-align: right;
}
</style>
<h1>ark-serman: an Ark Dedicated Server Manager</h1>
<p>
<table>
<thead>
<tr>
<th>Map</th>
<th>State</th>
<th>Command</th>
<th>Memory</th>
<th>CPU</th>
</tr>
</thead>
{{range .Servers}}
<tr>
<td>{{.DisplayName}}</td>
{{if .Running}}
<td><strong>{{.ActiveState}}</strong></td>
<td><form action="/rpc/stop/{{.Name}}" method="POST"><input type="submit" value="Stop"></form></td>
{{/* <td>{{range $k, $v := .Props}}{{$k}}: {{$v}}<br>{{end}}</td> */}}
<td>{{.CPU}} s</td>
<td>{{.Memory}} MiB</td>
{{else}}
<td>{{.ActiveState}}</td>
<td><form action="/rpc/start/{{.Name}}" method="POST"><input type="submit" value="Start"></form></td>
<td>N/A</td>
<td>N/A</td>
{{end}}
</tr>
{{end}}
</table>
`

var pageTmpl = template.Must(template.New("").Parse(page))

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
		unitNames = append(unitNames, path.Base(v.Path))
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

func main() {
	bind := flag.String("p", ":8070", "bind address and port")
	pwd := flag.String("pwd", "", "rcon password")
	quiet := flag.Bool("q", false, "don't print log lines")

	log.SetFlags(log.Lmicroseconds)
	flag.Parse()

	if flag.NArg() != 0 {
		if *pwd == "" {
			println("-pwd is required")
			os.Exit(1)
		}
		conn, err := rcon.Dial(*bind, *pwd)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()
		for _, a := range flag.Args() {
			fmt.Printf("Running: %s\n", a)
			resp, err := conn.Execute(a)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("  Got: %s\n", resp)
		}
		return
	} else if *pwd != "" {
		println("-pwd is unexpected")
		os.Exit(1)
	}

	mux := &http.ServeMux{}
	mux.Handle("/rpc/start/", http.HandlerFunc(rpcStart))
	mux.Handle("/rpc/stop/", http.HandlerFunc(rpcStop))
	mux.Handle("/", http.HandlerFunc(serveRoot))
	var h http.Handler = mux
	if !*quiet {
		h = &loghttp.Handler{Handler: mux}
	}
	s := &http.Server{
		Addr:           *bind,
		Handler:        h,
		ReadTimeout:    10. * time.Second,
		WriteTimeout:   60 * time.Second,
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
	}
	log.Printf("Serving on %s", *bind)
	log.Fatal(s.ListenAndServe())
}
