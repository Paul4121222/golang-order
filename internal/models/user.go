package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type User struct {
	ID int64 `json:"id"`
	Name string `json:"name"`
	Email string `json:"email"`
	PasswordHash string `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

//這寫法目的就是先把db存下來，之後每次呼叫方法都不用再傳db
type UserRepo struct {
	db *sql.DB
}

var ErrDuplicateEmail = errors.New("Email already exists.")

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) CreateUser(name, email, passwordHash string) (*User, error){
	query := `
		INSERT INTO users (name, email, password_hash)
		VALUES($1, $2, $3)
		RETURNING id, created_at
	`

	var u User
	u.Name = name
	u.PasswordHash = passwordHash
	u.Email = email

	err := r.db.QueryRow(query, name, email, passwordHash).Scan(&u.ID, &u.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("UserRepo.CreateUser: %w", err)
	}

	return &u, nil
}

func (r *UserRepo) GetUserByEmail(email string) (*User, error) {
	query := `
		SELECT id, name, email, created_at, password_hash
		FROM users WHERE email = $1
	`

	var u User
	err := r.db.QueryRow(query, email).Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.PasswordHash)

	//代表執行成功，只是沒資料
	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505"{
			return nil, ErrDuplicateEmail
		}

		return nil, fmt.Errorf("UserRepo.GetUserByEmail: %w", err)
	}

	return &u, nil
}

func (r *UserRepo) GetUserByID(userID int64) (*User, error) {
	query := `
		SELECT id, name, email, created_at, password_hash
		FROM users WHERE id = $1
	`

	var u User
	err := r.db.QueryRow(query, userID).Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return  nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("UserRepo.GetUserByID: %w", err)
	}

	return &u, nil
}