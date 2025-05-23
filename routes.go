package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

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
	router.HandleFunc("/transactions/{tokenID}", getTransactionsByTokenID).Methods("GET")
	router.HandleFunc("/current-tokens/{peerID}", getCurrentTokensByPeerID).Methods("GET")
	router.HandleFunc("/token-info/{tokenID}", getTokenInfoByTokenID).Methods("GET")
	// router.HandleFunc("/transactions/upsert", upsertTransactionHandler).Methods("POST")
	router.HandleFunc("/latesttoken", getLatestMintedToken).Methods("GET")
	router.HandleFunc("/synctokenstate/{tokenID}", syncLatestTokenState).Methods("GET")

	return router
}
