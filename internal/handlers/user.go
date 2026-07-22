package handlers

import (
	"errors"
	"net/http"
	"orders/internal/models"
	"strings"

	"github.com/gin-gonic/gin"
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
		if errors.Is(err, models.ErrDuplicateEmail) {
			c.JSON(409, gin.H{"error": "email已經註冊過"})
			return 
		}
		
		c.JSON(500, gin.H{"error": "internal error:" + err.Error()})

		return
	}

	if existing != nil {
		c.JSON(409, gin.H{"error": "email已經註冊過"})
		return
	}

	passwordHash := "fake_hash_" + req.Password

	user, err := u.repo.CreateUser(req.Name, email, passwordHash)
	if err != nil {
        c.JSON(500, gin.H{"error": "建立使用者失敗"})
        return
    }

	c.JSON(http.StatusCreated, user)

}