package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"decentralized-explorer-backend/ipfs"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/lib/pq"
)

var TokenMap = map[int]int{
	0:  0,
	1:  5000000,
	2:  2425000,
	3:  2303750,
	4:  2188563,
	5:  2079134,
	6:  1975178,
	7:  1876419,
	8:  1782598,
	9:  1693468,
	10: 1608795,
	11: 1528355,
	12: 1451937,
	13: 1379340,
	14: 1310373,
	15: 1244855,
	16: 1182612,
	17: 1123481,
	18: 1067307,
	19: 1013942,
	20: 963245,
	21: 915082,
	22: 869328,
	23: 825862,
	24: 784569,
	25: 745340,
	26: 708073,
	27: 672670,
	28: 639036,
	29: 607084,
	30: 576730,
	31: 547894,
	32: 520499,
	33: 494474,
	34: 469750,
	35: 446263,
	36: 423950,
	37: 402752,
	38: 382615,
	39: 363484,
	40: 345310,
	41: 328044,
	42: 311642,
	43: 296060,
	44: 281257,
	45: 267194,
	46: 253834,
	47: 241143,
	48: 229085,
	49: 217631,
	50: 206750,
	51: 196412,
	52: 186592,
	53: 177262,
	54: 168399,
	55: 159979,
	56: 151980,
	57: 144381,
	58: 137162,
	59: 130304,
	60: 117273,
	61: 105546,
	62: 94992,
	63: 85492,
	64: 76943,
	65: 69249,
	66: 62324,
	67: 56092,
	68: 50482,
	69: 45434,
	70: 40891,
	71: 36802,
	72: 33121,
	73: 29809,
	74: 26828,
	75: 24146,
	76: 21731,
	77: 19558,
	78: 17602,
}

func checkTokenCount() {
	// Get current token level and number
	currentLevel, currentNum := tokenNum()

	// Query the latest token from token_info
	var latestLevel, latestNum int
	err := db.QueryRow(`
        SELECT token_level, token_number 
        FROM token_info 
		WHERE token_type = 'RBT' 
        ORDER BY token_number DESC, token_level DESC
        LIMIT 1
    `).Scan(&latestLevel, &latestNum)

	if err != nil {
		if err == sql.ErrNoRows {
			// Table is empty - print True
			generateTokenID(currentLevel, currentNum, 1, 0)
			return
		}
		log.Printf("Error querying latest token: %v", err)
		return
	}

	// Check if current values are more than the latest in DB
	if currentLevel > latestLevel || currentNum > latestNum {
		generateTokenID(currentLevel, currentNum, latestLevel, latestNum)
	}

}

func generateTokenID(currentLevel int, currentNum int, latestLevel int, latestNum int) error {
	// Generate token ID based on current level and number
	ipfs := ipfs.GetShell()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Start from the latest existing token
	level := latestLevel
	num := latestNum + 1 // Start from next number

	for {
		// Check if we've reached the current level and number
		if level > currentLevel || (level == currentLevel && num > currentNum) {
			break
		}

		// Generate token ID - optimized version
		token_info := fmt.Sprintf("%d %d", level, num)

		// Add to IPFS - single allocation version
		token_id, err := ipfs.Add(strings.NewReader(token_info), ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		if err != nil {
			log.Printf("Failed to add token %q to IPFS: %v", token_info, err)
			return err
		}
		// Insert into token_info table
		_, err = tx.Exec(`
			INSERT INTO token_info 
			(token_id, token_level, token_number, token_value, parent_token_id, token_type)
			VALUES ($1, $2, $3, 1, NULL, RBT)
		`, token_id, level, num)
		if err != nil {
			return fmt.Errorf("failed to insert token %q: %w", token_info, err)
		}
		log.Printf("Added token %s to IPFS: %s", token_info, token_id)

		// Increment number
		num++

		// Check if we've reached max for current level
		if maxNum, exists := TokenMap[level]; exists {
			if num > maxNum {
				// Move to next level
				level++
				num = 1 // Reset number for new level

				// Check if level exists in our map
				if _, exists := TokenMap[level]; !exists {
					log.Printf("Reached maximum level %d", level-1)
					break
				}
			}
		} else {
			log.Printf("Invalid level %d in TokenMap", level)
			break
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func checkPins(token string) (*PinnerInfo, error) {
	currentWeek := GetWeeksPassed()
	ipfs := ipfs.GetShell()

	timestamp := time.Now()

	// Check pins for both tokenID and tokenEpochCID
	currentPinner, err := GetDHTddrs(token)
	if err != nil {
		log.Printf("Failed to check pins for token %s: %v", token, err)
		return nil, err
	}

	fmt.Printf("currentPinner : %v for token : %v", currentPinner, token)
	fmt.Println()

	// Check for ownership change -- update to pick the most recent pinner (TODO)

	if len(currentPinner) == 0 {
		return nil, fmt.Errorf("no peers found for token %s", token)
	}

	// Generate tokenEpoch hash (tokenID + weekEpoch)
	tokenEpoch := fmt.Sprintf("%s-%d", token, currentWeek)
	tokenEpochStr := bytes.NewReader([]byte(tokenEpoch))
	tokenEpochCID, err := ipfs.Add(tokenEpochStr, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))

	if err != nil {
		log.Printf("Failed to add token epoch %q to IPFS: %v", token, err)
		return nil, err
	}

	// fmt.Println("tokenEpochCID : ", tokenEpochCID)

	currentEpochPinner, err := GetDHTddrs(tokenEpochCID)
	if err != nil {
		log.Printf("Failed to check pins for token epoch %s: %v", tokenEpochCID, err)
		return nil, err
		// continue
	}

	// Begin database transaction
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if currentPinner != nil {
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM token_info WHERE token_id = $1)", token).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("failed to check token existence: %w", err)
		}

		if !exists {
			return &PinnerInfo{
				CurrentPinner:      currentPinner,
				CurrentEpochPinner: currentEpochPinner,
			}, nil
		}
	}

	var existingPeerIDs []string
	err = tx.QueryRow(`SELECT peer_ids FROM current_owners WHERE token_id = $1`, token).
		Scan(pq.Array(&existingPeerIDs))

	// If token exists in current_owners and peers haven't changed, skip update
	if err == nil && comparePeers(currentPinner, existingPeerIDs) {
		return nil, fmt.Errorf("no change in ownership for token %s", token)
	}

	t := Transaction{
		TokenID:   token,
		PeerID:    currentPinner,
		Epoch:     currentWeek,
		Quorums:   currentEpochPinner,
		Timestamp: timestamp,
	}

	err = upsertTransaction(t)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert transaction: %w", err)
	}
	return nil, nil
}

func GetDHTddrs(cid string) ([]string, error) {

	// Get the directory where the executable is located
	exeDir, err := getAppDir()
	if err != nil {
		return nil, err
	}

	// Use relative paths from the executable
	ipfsPath := filepath.Join(exeDir, "ipfs")
	repoPath := exeDir // Repo is in the same directory
	cmd := exec.Command(ipfsPath, "dht", "findprovs", cid)
	cmd.Env = append(os.Environ(), "IPFS_PATH="+repoPath)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open command stdout with err : %v", err)
	}
	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start command with err : %v", err)
	}
	ids := make([]string, 0)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		m := scanner.Text()
		if strings.Contains(m, "Error") {
			return nil, fmt.Errorf(m)
		}
		if !strings.HasPrefix(m, "Qm") {
			ids = append(ids, m)
		}
	}
	return ids, nil
}

func comparePeers(currentPinner []string, peerID []string) bool {
	if len(currentPinner) != len(peerID) {
		return false
	}
	peerSet := make(map[string]struct{})
	for _, peer := range peerID {
		peerSet[peer] = struct{}{}
	}

	for _, peer := range currentPinner {
		if _, exists := peerSet[peer]; !exists {
			return false
		}
	}
	return true
}

func syncMissingCurrentOwners() error {
	query := `
		SELECT token_id
		FROM token_info
		WHERE token_id NOT IN (SELECT token_id FROM current_owners)
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query orphan tokens: %w", err)
	}
	defer rows.Close()

	var missingTokens []string

	for rows.Next() {
		var tokenID string
		if err := rows.Scan(&tokenID); err != nil {
			log.Println("Row scan error:", err)
			continue
		}
		missingTokens = append(missingTokens, tokenID)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("row iteration error: %w", err)
	}

	if len(missingTokens) == 0 {
		log.Println("All tokens in token_info have current_owners")
		return nil
	}

	log.Printf("Found %d token(s) missing from current_owners. Checking pins...\n", len(missingTokens))
	for _, tokenID := range missingTokens {
		log.Printf("Checking pin for token_id: %s", tokenID)
		if _, err := checkPins(tokenID); err != nil {
			log.Printf("checkPins failed for %s: %v", tokenID, err)
		}
	}

	log.Println("syncMissingCurrentOwners complete")
	return nil
}

// func startDailyPinCheck1() {
// 	ticker := time.NewTicker(24 * time.Hour)
// 	defer ticker.Stop()
// 	for {
// 		log.Println("Daily pin check started...")
// 		rows, err := db.Query(`SELECT token_id FROM current_owners`)
// 		if err != nil {
// 			log.Println("DB query error in daily pin check:", err)
// 			continue
// 		}
// 		var count int
// 		for rows.Next() {
// 			var tokenID string
// 			if err := rows.Scan(&tokenID); err != nil {
// 				log.Println("Scan error in daily pin check:", err)
// 				continue
// 			}
// 			if err := checkPins(tokenID); err != nil {
// 				log.Printf("Pin check failed for %s: %v\n", tokenID, err)
// 			} else {
// 				log.Printf("Pin check OK for %s\n", tokenID)
// 			}
// 			count++
// 		}
// 		rows.Close()
// 		log.Printf("Daily pin check completed for %d tokens\n", count)
// 		<-ticker.C
// 	}
// }

func startDailyPinCheck() {
	// Add jitter (±1 hour) to avoid thundering herd
	jitter := time.Duration(rand.Int63n(int64(2*time.Hour))) - time.Hour
	ticker := time.NewTicker(24*time.Hour + jitter)
	defer ticker.Stop()

	for {
		startTime := time.Now()
		log.Println("Daily pin check started...")

		// Semaphore for controlling concurrency (e.g., 10 concurrent checks)
		sem := make(chan struct{}, 10)
		var wg sync.WaitGroup
		var processedCount int64
		var errorCount int64

		// Process in batches of 1000 tokens
		batchSize := 1000
		offset := 0

		for {
			// Get batch of tokens
			rows, err := db.Query(
				`SELECT token_id FROM current_owners ORDER BY token_id LIMIT $1 OFFSET $2`,
				batchSize, offset,
			)
			if err != nil {
				log.Printf("DB batch query error (offset %d): %v", offset, err)
				break
			}

			var batchCount int
			for rows.Next() {
				var tokenID string
				if err := rows.Scan(&tokenID); err != nil {
					log.Printf("Scan error in batch (offset %d): %v", offset, err)
					atomic.AddInt64(&errorCount, 1)
					continue
				}

				wg.Add(1)
				sem <- struct{}{} // Acquire semaphore slot
				go func(t string) {
					defer func() {
						<-sem // Release semaphore slot
						wg.Done()

						// Recover from any panic in checkPins
						if r := recover(); r != nil {
							log.Printf("Recovered from panic in token %s: %v", t, r)
							atomic.AddInt64(&errorCount, 1)
						}
					}()

					if _, err := checkPins(t); err != nil {
						log.Printf("Pin check failed for %s: %v", t, err)
						atomic.AddInt64(&errorCount, 1)
					}
					atomic.AddInt64(&processedCount, 1)
				}(tokenID)
				batchCount++
			}
			rows.Close()

			if batchCount == 0 {
				break // No more tokens
			}
			offset += batchCount
		}

		// Wait for all goroutines to finish
		wg.Wait()
		close(sem)

		// Log completion
		duration := time.Since(startTime)
		log.Printf(
			"Daily pin check completed. Tokens: %d, Errors: %d, Duration: %v",
			processedCount, errorCount, duration,
		)

		// Adjust next tick to ensure full 24h between completions
		if duration < 24*time.Hour {
			time.Sleep(24*time.Hour - duration)
		}
		<-ticker.C // Wait for next tick (with jitter)
	}
}
