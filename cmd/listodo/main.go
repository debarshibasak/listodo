package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/adaptive-scale/listodo/internal/desktop"
	"github.com/adaptive-scale/listodo/internal/sessions"
	"github.com/adaptive-scale/listodo/internal/store"
)

func main() {
	dataDir := flag.String("data-dir", "", "Override the local data folder")
	syncOnly := flag.Bool("sync-only", false, "Import local sessions and exit without opening a window")
	serve := flag.Bool("serve", false, "Serve the desktop interface in a browser without opening a native window")
	address := flag.String("listen", "127.0.0.1:0", "Loopback address for the local interface")
	flag.Parse()
	dir := *dataDir
	var err error
	if dir == "" {
		dir, err = store.DefaultDir()
		if err != nil {
			log.Fatal(err)
		}
	}
	s, err := store.Open(dir)
	if err != nil {
		log.Fatal(fmt.Errorf("cannot open task database: %w", err))
	}
	defer s.Close()
	if *syncOnly {
		settings, err := s.Settings()
		if err != nil {
			log.Fatal(err)
		}
		report := sessions.Scan(settings, time.Now())
		if err := s.SaveSessions(report.Sessions); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Imported %d sessions from %d transcript files.\n", len(report.Sessions), report.Files)
		for _, warning := range report.Warnings {
			fmt.Fprintln(os.Stderr, warning)
		}
		return
	}
	server, err := desktop.NewServer(s)
	if err != nil {
		log.Fatal(err)
	}
	server.Native = !*serve
	url, closeServer, err := server.Listen(*address)
	if err != nil {
		log.Fatal(err)
	}
	defer closeServer()
	if *serve {
		fmt.Printf("Listodo is running at %s\n", url)
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
		<-stop
		return
	}
	if err := desktop.RunWindow(url); err != nil {
		log.Print(err)
	}
}
