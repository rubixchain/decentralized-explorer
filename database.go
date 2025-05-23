package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

const (
	dbHost     = "localhost"
	dbPort     = 5432
	dbUser     = "postgres"
	dbPassword = "rubix"
	dbName     = "decentralized_explorer"
)

var db *sql.DB

func setupDatabase() error {
	// First attempt to connect directly to our target database
	targetConnStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName,
	)

	dbConn, err := sql.Open("postgres", targetConnStr)
	if err == nil {
		// Verify connection
		if err = dbConn.Ping(); err == nil {
			db = dbConn
			log.Println("Connected to existing database")
			return createSchema(db)
		}
		dbConn.Close()
	}

	// If we get here, we need to create the database
	log.Println("Database does not exist, creating it...")

	// Connect to postgres database to create our target db
	adminConnStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=postgres sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword,
	)

	adminDb, err := sql.Open("postgres", adminConnStr)
	if err != nil {
		return fmt.Errorf("error connecting to admin db: %w", err)
	}
	defer adminDb.Close()

	// Create the database
	if _, err = adminDb.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)); err != nil {
		return fmt.Errorf("failed to create database %s: %w", dbName, err)
	}

	// Now connect to the new database
	dbConn, err = sql.Open("postgres", targetConnStr)
	if err != nil {
		return fmt.Errorf("error connecting to new database %s: %w", dbName, err)
	}

	if err = dbConn.Ping(); err != nil {
		return fmt.Errorf("cannot ping new database: %w", err)
	}

	db = dbConn
	log.Println("Database created and connected successfully")
	return createSchema(db)
}

func createSchema(db *sql.DB) (retErr error) {
	// Start a transaction for all schema changes
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Use defer to rollback if we encounter any error
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	// Create tables
	_, retErr = tx.Exec(`

	-- New token_info table (master table for token metadata)
        CREATE TABLE IF NOT EXISTS token_info (
			token_id TEXT PRIMARY KEY,
			token_level INT NOT NULL,
			token_number INT NOT NULL,
			token_value NUMERIC NOT NULL,
			parent_token_id TEXT,
			-- Optional: Add constraint for parent-child relationship
			FOREIGN KEY (parent_token_id) REFERENCES token_info(token_id)
        );

		CREATE TABLE IF NOT EXISTS transactions (
			tx_id SERIAL PRIMARY KEY,
			token_id TEXT NOT NULL,
			peer_ids TEXT[] NOT NULL,  -- Changed from peer_id to array of strings
			epoch INT NOT NULL,
			quorums TEXT[] NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),  -- Added timestamp field
			FOREIGN KEY (token_id) REFERENCES token_info(token_id)
			-- CONSTRAINT uniq_token_peer_epoch UNIQUE (token_id, peer_id, epoch)
		);

		CREATE TABLE IF NOT EXISTS current_owners (
			token_id TEXT PRIMARY KEY,
			peer_ids TEXT[] NOT NULL,  -- Changed from peer_id to array of strings
			epoch INT NOT NULL,
			quorums TEXT[] NOT NULL,	
			timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),  -- Added timestamp field
			FOREIGN KEY (token_id) REFERENCES token_info(token_id)
		);
	`)
	if retErr != nil {
		return fmt.Errorf("failed to create tables: %w", retErr)
	}

	// Create indexes
	_, retErr = tx.Exec(`
	    CREATE INDEX IF NOT EXISTS idx_token_info ON token_info(token_id);
		CREATE INDEX IF NOT EXISTS idx_token_id ON transactions (token_id);
		CREATE INDEX idx_current_owners_timestamp ON current_owners(timestamp DESC);
		--CREATE INDEX IF NOT EXISTS idx_peer_id ON transactions (peer_ids);
		-- CREATE INDEX IF NOT EXISTS idx_epoch ON transactions (epoch);
		--CREATE INDEX IF NOT EXISTS idx_current_owner_peer_id ON current_owners (peer_ids);

		-- GIN indexes for array operations (enables efficient array queries)
		CREATE INDEX IF NOT EXISTS idx_transactions_peer_ids ON transactions USING GIN(peer_ids);
		CREATE INDEX IF NOT EXISTS idx_current_owner_peer_ids ON current_owners USING GIN(peer_ids);
	`)
	if retErr != nil {
		return fmt.Errorf("failed to create indexes: %w", retErr)
	}

	// Commit the transaction
	if retErr = tx.Commit(); retErr != nil {
		return fmt.Errorf("failed to commit transaction: %w", retErr)
	}

	log.Println("Database schema created successfully")
	return nil
}

func upsertTransaction(t Transaction) error {
	fmt.Println("Upserting transaction:", t)

	// Verify database connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	var existingPeerIDs []string
	var existingEpoch int
	var existingQuorums []string
	var existingUpdatedAt time.Time

	//Can use later for comparison
	err = tx.QueryRow(`SELECT peer_ids, epoch, quorums, timestamp FROM current_owners WHERE token_id = $1`, t.TokenID).
		Scan(pq.Array(&existingPeerIDs), &existingEpoch, pq.Array(&existingQuorums), &existingUpdatedAt)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error checking existing current_owner: %w", err)
	}

	// If no existing row → insert both into current_owners and transactions
	if err == sql.ErrNoRows {
		fmt.Println("No existing row found, inserting new token_id:", t.TokenID)
		res, err := tx.Exec(`
				INSERT INTO current_owners (token_id, peer_ids, epoch, quorums, timestamp)
				VALUES ($1, $2, $3, $4, $5)
			`, t.TokenID, pq.Array(&t.PeerID), t.Epoch, pq.Array(&t.Quorums), t.Timestamp)
		if err != nil {
			return fmt.Errorf("failed to insert into current_owners: %w", err)
		}

		rowsAffected, _ := res.RowsAffected()
		fmt.Printf("Inserted into current_owners: %d rows affected\n", rowsAffected)

		_, err = tx.Exec(`
				INSERT INTO transactions (token_id, peer_ids, epoch, quorums, timestamp)
				VALUES ($1, $2, $3, $4, $5)
			`, t.TokenID, pq.Array(&t.PeerID), t.Epoch, pq.Array(&t.Quorums), t.Timestamp)
		if err != nil {
			return fmt.Errorf("failed to insert into transactions: %w", err)
		}

		fmt.Printf("🆕 New token_id %s added to current_owners and transactions", t.TokenID)
	} else {
		_, err = tx.Exec(`
					UPDATE current_owners
					SET peer_ids = $1, epoch = $2, quorums = $3, timestamp = $4
					WHERE token_id = $5
				`, pq.Array(&t.PeerID), t.Epoch, pq.Array(&t.Quorums), t.Timestamp, t.TokenID)
		if err != nil {
			return fmt.Errorf("failed to update current_owners: %w", err)
		}

		// Insert new transaction
		_, err = tx.Exec(`
					INSERT INTO transactions (token_id, peer_ids, epoch, quorums, timestamp)
					VALUES ($1, $2, $3, $4, $5)
				`, t.TokenID, pq.Array(&t.PeerID), t.Epoch, pq.Array(&t.Quorums), t.Timestamp)
		if err != nil {
			return fmt.Errorf("failed to insert into transactions (update flow): %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	log.Println("Upsert completed")
	return nil
}
