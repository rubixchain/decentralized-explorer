package ipfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
)

var (
	sh      *shell.Shell
	once    sync.Once
	ipfsAPI string = "localhost:5001" // or from config
)

func GetShell() *shell.Shell {
	once.Do(func() {
		sh = shell.NewShell(ipfsAPI)
	})
	return sh
}

// func SetAPI(api string) {
// 	ipfsAPI = api
// }

func NewIPFSSetup(appDir string) error {
	// Use relative paths from the executable
	//repo path is same as appDir
	ipfsPath := filepath.Join(appDir, "ipfs")

	// Check if repo already exists
	if _, err := os.Stat(filepath.Join(appDir, "config")); err == nil {
		return nil // Repo already initialized
	}

	//1. Initialize repo with identical CLI settings
	cmd := exec.Command(ipfsPath, "init")
	cmd.Env = append(os.Environ(), "IPFS_PATH="+appDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize IPFS repo: %w", err)
	}

	// 2. Apply all configurations including custom ones
	configs := []struct {
		key   string
		value string
		json  bool
	}{
		// Basic configurations (matches CLI)
		{"Addresses.API", "/ip4/127.0.0.1/tcp/5001", false},
		{"Addresses.Gateway", "/ip4/127.0.0.1/tcp/8080", false},
		{"Addresses.Swarm", `["/ip4/0.0.0.0/tcp/4001", "/ip6/::/tcp/4001"]`, true},
		{"Swarm.EnablePubsubExperiment", "true", false},
		{"Routing.Type", "dht", false},

		// Your custom configurations
		// {"Experimental.Libp2pStreamMounting", "true", false},
		{"Bootstrap", `[
            "/ip4/103.209.145.177/tcp/4001/p2p/12D3KooWD8Rw7Fwo4n7QdXTCjbh6fua8dTqjXBvorNz3bu7d9xMc",
            "/ip4/98.70.52.158/tcp/4001/p2p/12D3KooWQyWFABF3CKFnzX85hf5ZwrT5zPsy4rWHdGPZ8bBpRVCK"
        ]`, true},
	}

	for _, cfg := range configs {
		var cmd *exec.Cmd
		if cfg.json {
			cmd = exec.Command(ipfsPath, "config", "--json", cfg.key, cfg.value)
		} else {
			cmd = exec.Command(ipfsPath, "config", cfg.key, cfg.value)
		}

		cmd.Env = append(os.Environ(), "IPFS_PATH="+appDir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("config %s failed: %w", cfg.key, err)
		}
	}

	return nil
}

func StartDaemon(repoPath string) (*exec.Cmd, error) {
	cmd, err := startipfsdaemon(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to start IPFS daemon: %w", err)
	}
	time.Sleep(20 * time.Second)

	//Here add the config {"Experimental.Libp2pStreamMounting", "true", false}
	ipfsPath := filepath.Join(repoPath, "ipfs")
	getConfigCmd := exec.Command(ipfsPath, "config", "--json", "Experimental.Libp2pStreamMounting")
	getConfigCmd.Env = append(os.Environ(), "IPFS_PATH="+repoPath)
	output, err := getConfigCmd.CombinedOutput()
	if err == nil && strings.TrimSpace(string(output)) == "true" {
		// Config is already enabled, return the running daemon
		return cmd, nil
	}

	// First set the Libp2pStreamMounting config
	setConfigCmd := exec.Command(ipfsPath, "config", "--json", "Experimental.Libp2pStreamMounting", "true")
	setConfigCmd.Env = append(os.Environ(), "IPFS_PATH="+repoPath)
	if err := setConfigCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to set Libp2pStreamMounting: %w", err)
	}
	err = stopipfsdaemon(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to stop IPFS daemon: %w", err)
	}

	// Step 4: Restart the daemon
	cmd, err = startipfsdaemon(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to restart IPFS daemon: %w", err)
	}

	time.Sleep(20 * time.Second)

	return cmd, nil
}

func startipfsdaemon(repoPath string) (*exec.Cmd, error) {
	ipfsPath := filepath.Join(repoPath, "ipfs")

	cmd := exec.Command(ipfsPath, "daemon", "--enable-pubsub-experiment")
	cmd.Env = append(os.Environ(), "IPFS_PATH="+repoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start daemon: %w", err)
	}

	return cmd, nil
}

func stopipfsdaemon(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("daemon process not running")
	}

	// Try graceful shutdown first
	err := cmd.Process.Kill()
	if err != nil {
		return fmt.Errorf("failed to send interrupt signal: %w", err)
	}
	return nil
}
