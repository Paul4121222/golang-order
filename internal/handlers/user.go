package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"orders/internal/auth"
	"orders/internal/models"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	repo *models.UserRepo
}

func NewUserHandler (repo *models.UserRepo) *UserHandler {
	return &UserHandler{repo: repo}
}

type RegisterUser struct {
	Name string `json:"name" binding:"required,min=1"`
	Email string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

func (u *UserHandler) Register(c *gin.Context) {
	var req RegisterUser
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400,gin.H{"error": "資料格式不正確:" + err.Error()})
		return
	}

	email := strings.ToLower(req.Email)

	existing, err := u.repo.GetUserByEmail(email)

	if err != nil {
		c.JSON(500, gin.H{"error": "internal error:" + err.Error()})

		return
	}

	if existing != nil {
		c.JSON(409, gin.H{"error": "email已經註冊過"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {

        c.JSON(500, gin.H{"error": "hash 密碼失敗"})
        return
    }

	user, err := u.repo.CreateUser(req.Name, email, string(hash))

	if err != nil {
		if errors.Is(err, models.ErrDuplicateEmail) {
			c.JSON(409, gin.H{"error": "email已經註冊過"})
			return 
		}
		
        c.JSON(500, gin.H{"error": "建立使用者失敗"})
        return
    }

	c.JSON(http.StatusCreated, user)

}

type LoginUser struct {
	Email string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=1"`
}

func (u *UserHandler) Login(c *gin.Context) {
	var req LoginUser
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "資料格式不正確"+ err.Error()})
		return
	}

	email := strings.ToLower(req.Email)

	user, err := u.repo.GetUserByEmail(email)
	if err != nil {
		c.JSON(500, gin.H{"error": "internal error"})
		return
	}

	if user == nil {
		c.JSON(401, gin.H{"error": "帳號或密碼錯誤"})
        return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
        c.JSON(401, gin.H{"error": "帳號或密碼錯誤"})
        return
    }

	token, err := auth.GenerateToken(user.ID)

	if err != nil {
        c.JSON(500, gin.H{"error": "產生 token 失敗"})
        return
    }

	c.JSON(200, gin.H{
		"user":user,
		"token":token,
	})
}

func (u *UserHandler) Me(c *gin.Context) {
	userID := c.GetInt64("userID")

	user, err := u.repo.GetUserByID(userID)

	if err != nil {
		fmt.Println(err.Error())
        c.JSON(500, gin.H{"error": "internal error"})
        return
    }

	if user == nil {
		c.JSON(404, gin.H{"error": "使用者不存在"})
        return
	}

	c.JSON(200, user)   // password_hash 自動被排除
}