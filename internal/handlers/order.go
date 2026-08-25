package handlers

import (
	"orders/internal/models"
	"strconv"

	"errors"

	"github.com/gin-gonic/gin"
)

type OrderHandler struct {
	repo *models.OrderRepo
}

func NewOrderHandler(db *models.OrderRepo) *OrderHandler {
	return &OrderHandler{repo: db}
}


//有dive，就會深入檢查slice的item是否有符合
type OrderInput struct{
	Items []models.OrderInput `json:"items" binding:"required,min=1,dive"`
}

func (o OrderHandler) Create(c *gin.Context) {
	var input OrderInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "資料格式不正確:" + err.Error()})
		return
	}

	userID := c.GetInt64("userID")

	order, err := o.repo.CreateOrder(userID, input.Items)
	if err != nil {
		if errors.Is(err, models.ErrProductNotFound) {
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, models.ErrInsufficientStock) {
            c.JSON(409, gin.H{"error": err.Error()})
            return
        }
        c.JSON(500, gin.H{"error": "建立訂單失敗"})
        return
	}

	 c.JSON(201, order)
}

func (o OrderHandler) Get(c *gin.Context) {
	userID := c.GetInt64("userID")

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
        c.JSON(400, gin.H{"error": "id 必須是數字"})
        return
    }

	order, err := o.repo.GetOrderByID(id, userID)
	if err != nil {
        c.JSON(500, gin.H{"error": "internal error"})
        return
    }
    if order == nil {
        c.JSON(404, gin.H{"error": "找不到訂單"})
        return
    }

    c.JSON(200, order)
}

func (o *OrderHandler) List(c *gin.Context) {
    userID := c.GetInt64("userID")

    orders, err := o.repo.ListOrderByUserID(userID)
    if err != nil {
        c.JSON(500, gin.H{"error": "internal error"})
        return
    }

    c.JSON(200, gin.H{"orders": orders})
}

func (o *OrderHandler) GetFullOrders(c *gin.Context) {
	userID := c.GetInt64("userID")

	collections, err := o.repo.ListOrdersWithItemsByUserID(userID)

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"collections": collections,
	})
}
