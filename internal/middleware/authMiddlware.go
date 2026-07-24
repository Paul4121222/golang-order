package middleware

import (
	"orders/internal/auth"
	"strings"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware() gin.HandlerFunc{
	return func (c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "未提供認證"})
			return 
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(401, gin.H{"error": "認證格式有誤"})
			return
		}

		userID, err := auth.ParseToken(parts[1])

		if err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "認證無效"})
            return
        }

		c.Set("userID", userID)
		c.Next()
	}
}