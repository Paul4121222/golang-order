package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func New(connStr string) (*sql.DB, error) {
	db, err := sql.Open("pgx", connStr)

	if err != nil {
		//%w 保留error物件
		return nil, fmt.Errorf("開啟連線失敗: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(50 * time.Second)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Ping失敗: %w", err)
	}
	
	return db, nil
}