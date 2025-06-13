package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
)

type Transaction struct {
	TxID      int       `json:"tx_id"`
	TokenID   string    `json:"token_id"`
	PeerID    []string  `json:"peer_id"`
	Epoch     int       `json:"epoch"`
	Quorums   []string  `json:"quorums"`
	Timestamp time.Time `json:"timestamp"`
}

type CurrentOwner struct {
	TokenID   string    `json:"token_id"`
	PeerID    []string  `json:"peer_id"`
	Epoch     int       `json:"epoch"`
	Quorums   []string  `json:"quorums"`
	Timestamp time.Time `json:"timestamp"`
}

type TokenInfo struct {
	TokenID       string  `json:"token_id"`
	TokenLevel    int     `json:"token_level"`
	TokenNumber   int     `json:"token_number"`
	TokenValue    float64 `json:"token_value"`
	TokenType     string  `json:"token_type"`
	ParentTokenID string  `json:"parent_token_id"`
}

type PinnerInfo struct {
	TokenDetails       string   `json:"tokenDetails"`
	CurrentPinner      []string `json:"currentPinner"`
	CurrentEpochPinner []string `json:"currentEpochPinner"`
}

func getTransactionsByTokenID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	tokenID := vars["tokenID"]

	// Check if token exists first
	var tokenExists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM token_info WHERE token_id = $1)", tokenID).Scan(&tokenExists)
	if err != nil {
		http.Error(w, fmt.Sprintf("Token check error: %v", err), http.StatusInternalServerError)
		return
	}

	if !tokenExists {
		pinnerInfo, _ := checkPins(tokenID)
		if pinnerInfo != nil {
			// Token not found, returning just pinner info
			json.NewEncoder(w).Encode(map[string]interface{}{
				"tokenDetails":       pinnerInfo.TokenDetails,
				"currentPinner":      pinnerInfo.CurrentPinner,
				"currentEpochPinner": pinnerInfo.CurrentEpochPinner,
				"isExists":           tokenExists,
			})
			return
		}
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}

	page := 1
	limit := 30 // default

	queryParams := r.URL.Query()
	if val := queryParams.Get("page"); val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			page = p
		}
	}
	if val := queryParams.Get("limit"); val != "" {
		if l, err := strconv.Atoi(val); err == nil && l > 0 {
			// if l > 100 {
			// 	l = 100
			// }
			limit = l
		}
	}

	offset := (page - 1) * limit

	var totalCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE token_id = $1`, tokenID).Scan(&totalCount)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get total count: %v", err), http.StatusInternalServerError)
		return
	}

	if totalCount == 0 {
		http.Error(w, "No details found for tokenID", http.StatusNotFound)
		return
	}

	query := `
		SELECT tx_id, token_id, peer_ids, epoch, quorums, timestamp
		FROM transactions
		WHERE token_id = $1
		ORDER BY timestamp DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := db.Query(query, tokenID, limit, offset)
	if err != nil {
		http.Error(w, fmt.Sprintf("DB query error: %v", err), http.StatusInternalServerError)
		log.Println("DB query error:", err)
		return
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var t Transaction

		err := rows.Scan(&t.TxID, &t.TokenID, pq.Array(&t.PeerID), &t.Epoch, pq.Array(&t.Quorums), &t.Timestamp)
		if err != nil {
			http.Error(w, "DB row scan error", http.StatusInternalServerError)
			log.Println("Row scan error:", err)
			return
		}

		transactions = append(transactions, t)
	}

	if err = rows.Err(); err != nil {
		http.Error(w, "Rows iteration error", http.StatusInternalServerError)
		log.Println("Rows iteration error:", err)
		return
	}

	// Enhanced response with pagination metadata
	response := map[string]interface{}{
		"isExists": tokenExists,
		"data":     transactions,
		"pagination": map[string]interface{}{
			"total":        totalCount,
			"current_page": page,
			"per_page":     limit,
			"total_pages":  int(math.Ceil(float64(totalCount) / float64(limit))),
		},
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		log.Println("JSON encoding error:", err)
	}
}

func getCurrentTokensByPeerID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	peerID := vars["peerID"]

	page := 1
	limit := 30 // default

	queryParams := r.URL.Query()
	if val := queryParams.Get("page"); val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			page = p
		}
	}
	if val := queryParams.Get("limit"); val != "" {
		if l, err := strconv.Atoi(val); err == nil && l > 0 {
			// if l > 100 {
			// 	l = 100
			// }
			limit = l
		}
	}

	offset := (page - 1) * limit
	var tokenOwnedCount int

	err := db.QueryRow(`SELECT COUNT(*) FROM current_owners WHERE $1 = ANY(peer_ids)`, peerID).Scan(&tokenOwnedCount)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get total count of tokens: %v", err), http.StatusInternalServerError)
		return
	}

	if tokenOwnedCount == 0 {
		http.Error(w, "No tokens found for peerID "+peerID, http.StatusNotFound)
		return
	}

	query := `
		SELECT co.token_id, co.peer_ids, co.epoch, co.quorums, co.timestamp, ti.token_value
		FROM current_owners co
		JOIN token_info ti ON co.token_id = ti.token_id
		WHERE $1 = ANY(co.peer_ids)
		ORDER BY co.timestamp DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := db.Query(query, peerID, limit, offset)
	if err != nil {
		http.Error(w, "DB query error", http.StatusInternalServerError)
		log.Println("DB query error:", err)
		return
	}
	defer rows.Close()

	totalValue := 0.0
	var tokensOwned []CurrentOwner
	for rows.Next() {
		var token CurrentOwner
		var tokenValue float64

		err := rows.Scan(&token.TokenID, pq.Array(&token.PeerID), &token.Epoch, pq.Array(&token.Quorums), &token.Timestamp, &tokenValue)
		if err != nil {
			http.Error(w, "DB row scan error", http.StatusInternalServerError)
			log.Println("Row scan error:", err)
			return
		}
		totalValue += tokenValue

		tokensOwned = append(tokensOwned, token)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Rows iteration error", http.StatusInternalServerError)
		log.Println("Rows iteration error:", err)
		return
	}

	// Enhanced response with pagination metadata
	response := map[string]interface{}{
		"data":        tokensOwned,
		"total_value": totalValue,
		"pagination": map[string]interface{}{
			"total":        tokenOwnedCount,
			"current_page": page,
			"per_page":     limit,
			"total_pages":  int(math.Ceil(float64(tokenOwnedCount) / float64(limit))),
		},
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		log.Println("JSON encoding error:", err)
	}
}

func getTokenInfoByTokenID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tokenID := vars["tokenID"]
	var parentTokenID sql.NullString

	query := `
		SELECT token_level, token_number, token_value, parent_token_id, token_type
		FROM token_info
		WHERE token_id = $1
	`

	rows, err := db.Query(query, tokenID)
	if err != nil {
		http.Error(w, "DB query error", http.StatusInternalServerError)
		log.Println("DB query error:", err)
		return
	}
	defer rows.Close()

	var t TokenInfo
	for rows.Next() {
		err := rows.Scan(&t.TokenLevel, &t.TokenNumber, &t.TokenValue, &parentTokenID, &t.TokenType)
		if err != nil {
			http.Error(w, "DB row scan error", http.StatusInternalServerError)
			log.Println("Row scan error:", err)
			return
		}

		if parentTokenID.Valid {
			t.ParentTokenID = parentTokenID.String
		} else {
			t.ParentTokenID = "" // or "null" or "<none>" if you prefer
		}

		t.TokenID = tokenID
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

// func upsertTransactionHandler(w http.ResponseWriter, r *http.Request) {
// 	var t Transaction
// 	err := json.NewDecoder(r.Body).Decode(&t)
// 	if err != nil {
// 		http.Error(w, "Invalid request payload", http.StatusBadRequest)
// 		log.Println("Decode error:", err)
// 		return
// 	}
// 	err = upsertTransaction(t)
// 	if err != nil {
// 		http.Error(w, "Upsert failed: "+err.Error(), http.StatusInternalServerError)
// 		log.Println("Upsert error:", err)
// 		return
// 	}
// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
// }

func syncLatestTokenState(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tokenID := vars["tokenID"]

	_, err := checkPins(tokenID)
	if err != nil {
		http.Error(w, "Sync failed: "+err.Error(), http.StatusInternalServerError)
		log.Println("Sync error:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func getLatestMintedToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tokenLevel, tokenNumber := tokenNum()
	response := map[string]interface{}{
		"token_number": tokenNumber,
		"token_level":  tokenLevel,
	}
	json.NewEncoder(w).Encode(response)
}

func tokenNum() (int, int) {
	// Placeholder logic for token number and level
	// In a real application, this would involve querying the database or some other logic
	// to determine the current token number and level.
	return 4, 2188563
}

func getCurrentTokens(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Default pagination values
	page := 1
	limit := 50

	// Parse query parameters
	query := r.URL.Query()
	if val := query.Get("page"); val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			page = p
		}
	}
	if val := query.Get("limit"); val != "" {
		if l, err := strconv.Atoi(val); err == nil && l > 0 {
			// if l > 100 {
			// 	l = 100
			// }
			limit = l
		}
	}

	offset := (page - 1) * limit

	// Get total count for pagination metadata
	var totalCount int
	err := db.QueryRow("SELECT COUNT(*) FROM current_owners").Scan(&totalCount)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get total count: %v", err), http.StatusInternalServerError)
		return
	}

	// Query to fetch current owners with pagination and order
	rows, err := db.Query(`
		SELECT token_id, peer_ids, epoch, quorums, timestamp
		FROM current_owners
		ORDER BY timestamp DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		http.Error(w, "Database query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []CurrentOwner

	for rows.Next() {
		var owner CurrentOwner
		err := rows.Scan(&owner.TokenID, pq.Array(&owner.PeerID), &owner.Epoch, pq.Array(&owner.Quorums), &owner.Timestamp)
		if err != nil {
			http.Error(w, "Failed to scan result: "+err.Error(), http.StatusInternalServerError)
			return
		}
		results = append(results, owner)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Row iteration error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Enhanced response with pagination metadata
	response := struct {
		Data       []CurrentOwner `json:"data"`
		Pagination struct {
			Total       int `json:"total"`
			CurrentPage int `json:"current_page"`
			PerPage     int `json:"per_page"`
			TotalPages  int `json:"total_pages"`
		} `json:"pagination"`
	}{
		Data: results,
		Pagination: struct {
			Total       int `json:"total"`
			CurrentPage int `json:"current_page"`
			PerPage     int `json:"per_page"`
			TotalPages  int `json:"total_pages"`
		}{
			Total:       totalCount,
			CurrentPage: page,
			PerPage:     limit,
			TotalPages:  int(math.Ceil(float64(totalCount) / float64(limit))),
		},
	}

	json.NewEncoder(w).Encode(response)
}
