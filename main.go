package main

import (
	"decentralized-explorer-backend/ipfs"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {

	appDir, err := getAppDir()
	if err != nil {
		fmt.Printf("Error getting application directory: %v\n", err)
		os.Exit(1)
	}

	// Setup IPFS environment
	err = ipfs.NewIPFSSetup(appDir)
	// if err := ipfsSetup.EnsureIPFS();
	if err != nil {
		fmt.Printf("IPFS setup failed: %v\n", err)
		os.Exit(1)
	}

	daemonCmd, err := ipfs.StartDaemon(appDir)
	if err != nil {
		log.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemonCmd.Process.Kill()

	if err := setupDatabase(); err != nil {
		log.Fatal("Database setup failed:", err)
	}
	defer db.Close()

	router := setupRoutes()

	// checkTokenCount()
	// if err := syncMissingCurrentOwners(); err != nil {
	// 	log.Println("Error during initial sync:", err)
	// }

	// Periodic weekly sync to check newly minted tokens(runs in the background)
	go startWeeklySync()
	go startDailyPinCheck()
	// err = checkPins("QmQPG1tw3TqEbQGvs8AS89LWNsWmn9zzoPcyZbSPucdXne")
	// if err != nil {
	// 	log.Println("Error checking pins:", err)
	// }

	log.Println("Server started on :3000")
	log.Fatal(http.ListenAndServe(":3000", router))

}
