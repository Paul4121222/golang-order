package handlers

import (
	"orders/internal/models"

	"github.com/gin-gonic/gin"
)

type OrderHandler struct {
	db *models.OrderRepo
}

func NewOrderHandler(repo *models.OrderRepo) *OrderHandler{
	return &OrderHandler{db: repo}
}

func (r *OrderHandler) GetFullOrders(c *gin.Context) {
	userID := c.GetInt64("userID")

	collections, err := r.db.ListOrdersWithItemsByUserID(userID)

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"collections": collections,
	})
}