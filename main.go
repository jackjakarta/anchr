package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jackjakarta/anchr/config"
	"github.com/jackjakarta/anchr/s3client"
	"github.com/jackjakarta/anchr/ui"
)

var version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "print version")
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("anchr %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	clients := make([]*s3client.Client, len(cfg.Buckets))
	for i, b := range cfg.Buckets {
		client, err := s3client.NewClient(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating client for %q: %s\n", b.Name, err)
			os.Exit(1)
		}
		clients[i] = client
	}

	model := ui.NewModel(cfg, clients)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
