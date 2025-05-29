package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

// CORS middleware
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow all origins — for dev only, restrict in production!
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func setupRoutes() *mux.Router {
	router := mux.NewRouter()

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Server is up and running 🚀")
	})

	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})

	router.HandleFunc("/current-tokens", getCurrentTokens).Methods("GET")
	// router.HandleFunc("/token-info", getTokenInfo).Methods("GET")
	router.HandleFunc("/token-updates/{tokenID}", getTransactionsByTokenID).Methods("GET")
	router.HandleFunc("/current-tokens/{peerID}", getCurrentTokensByPeerID).Methods("GET")
	router.HandleFunc("/token-info/{tokenID}", getTokenInfoByTokenID).Methods("GET")
	// router.HandleFunc("/transactions/upsert", upsertTransactionHandler).Methods("POST")
	router.HandleFunc("/latesttoken", getLatestMintedToken).Methods("GET")
	router.HandleFunc("/synctokenstate/{tokenID}", syncLatestTokenState).Methods("GET")

	return router
}
