package main

import (
	"log"
	"orders/internal/database"
	"orders/internal/handlers"
	"orders/internal/middleware"
	"orders/internal/models"

	"github.com/gin-gonic/gin"
)




func main() {
	connStr := "postgres://paul:secret123@localhost:5432/orderdb?sslmode=disable"
	db, err := database.New(connStr)
	
	if err != nil {
        log.Fatal(err)
    }
	log.Println("DB 連上")
	defer db.Close()
	
	userRepo := models.NewUserRepo(db)

	userHandler := handlers.NewUserHandler(userRepo)

	orderRepo := models.NewOrderDb(db)
	orderHandler := handlers.NewOrderHandler(orderRepo)

	r := gin.Default()

	api := r.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
            c.JSON(200, gin.H{"status": "ok"})
        })
		api.POST("/users/register", userHandler.Register)
		api.POST("/users/login", userHandler.Login)

		//私有
		auth := api.Group("")
		auth.Use(middleware.AuthMiddleware())
		{
			auth.GET("/users/me", userHandler.Me)

			auth.GET("/orders/full", orderHandler.GetFullOrders)
		}
	}

	r.NoRoute(func(c *gin.Context) {
        c.JSON(404, gin.H{"error": "路徑不存在"})
    })

	log.Println("Server starting on :8080")
	r.Run(":8080")
}