import (
	"time"
	"database/sql"
	"errors"
)

var ErrProductNotFound = errors.New("Product Not Found.")
var ErrInsufficientStock = errors.New("insufficient Stock.")

type Order struct {
	ID int64 `json:"id"`
	UserID int64 `json:"user_id"`
	TotalCents int64 `json:"total_cents"`
	Status string `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Items []OrderItem `json:"items,omitempty"`
}

type OrderItem struct {
	ID int64 `json:"id"`
	OrderID        int64  `json:"order_id"`
    ProductID      int64  `json:"product_id"`
	ProductName    string `json:"product_name,omitempty"`
	Quantity       int    `json:"quantity"`
    UnitPriceCents int64  `json:"unit_price_cents"`
}

type OrderRepo struct {
	db *sql.DB
}

func NewOrderDb(db *sql.DB) *OrderRepo{
	return &OrderRepo{db: db}
}

//使用者的訂單內容
type OrderInput struct {
	Product_id string `binding: "required"`
	Quantity int `binding: "required,gte=1,lte=100"`
}

func (o *OrderRepo) CreateOrder(userID int64, items []OrderInput) (*Order, error){
	orderItems := make([]OrderItem, 0, len(items))
	var totalCents int64 

	//建立Transaction，只有全部DB操作成功才套用，其中一步失敗就整個操作不算。
	tx, err := tx.Begin()
	if err != nil {
        return nil, fmt.Errorf("OrderRepo.CreateOrder: begin: %w", err)
    }

	defer tx.Rollback()

	//Step1: 查product DB
	for _, input := range(items) {
		var productName string
		var priceCents int64
		var stock int64

		err := tx.QueryRow(`
			SELECT name, price_cents, stock FROM products WHERE id = $1
		`, input.Product_id).Scan(&productName, &priceCents, &stock)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: productID = %d", ErrProductNotFound, input.Product_id)
		}

		if err != nil {
			return nil, fmt.Errorf("OrderRepo.CreateOrder: query product %w", err)
		}

		if input.Quantity > stock {
			return nil, fmt.Errorf("%w: productID = %d", ErrInsufficientStock, input.Product_id)
		}

		totalCents += priceCents * int64(input.Quantity)
		orderItems = append(orderItems, OrderItem{
			ProductID:      input.ProductID,
            ProductName:    productName,
            Quantity:       input.Quantity,
            UnitPriceCents: priceCents,
		})
	}
	

	//Step2: Insert Order
	var order Order
	order.UserID = userID
	order.TotalCents = totalCents
	order.Status = "pending"

	err := tx.QueryRow(`
		INSERT INTO orders(user_id, total_cents, status) VALUES($1,$2,$3)
		RETURNING  id, created_at
	`, userID, totalCents, "pending").Scan(&order.ID, &order.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("OrderRepo.CreateOrder: Insert Order %w", err)
	}

	//Step3: Insert OrderItem and Update stock
	for i := range(orderItems) {
		orderItems[i].OrderID = order.ID

		var id int64
		err := tx.QueryRow(`
			INSERT INTO order_items(order_id, product_id, quantity, unit_price_cents) 
			VALUES($1,$2,$3,$4)
		`, orderItems[i].OrderID, orderItems[i].ProductID, orderItems[i].Quantity, orderItems[i].UnitPriceCents).Scan(&id)

		if err != nil {
            return nil, fmt.Errorf("OrderRepo.CreateOrder: insert item: %w", err)
        }
		orderItems[i].ID = id

		if _, err := tx.Exec(`
			UPDATE products SET stock = stock - $1 WHERE = id = $2
		`, orderItems[i].Quantity, orderItems[i].ProductID)

		if err != nil {
            return nil, fmt.Errorf("OrderRepo.CreateOrder: update stock: %w", err)
        }
	}

	//Step4: commit
	if err := tx.Commit(); err != nil {
        return nil, fmt.Errorf("OrderRepo.CreateOrder: commit: %w", err)
    }

	order.Items = orderItems
	return &order, nil
}

func (o *OrderRepo) GetOrderByID(orderID, userID int64) (*Order, error) {
	var order Order
	err := o.db.QueryRow(`
		SELECT id, user_id, total_cents, status created_at 
		FROM orders WHERE id = $1 AND user_id = $2
	`, orderID, userID).Scan(&order.ID, &order.UserID, &order.TotalCents, &order.Status, &order.CreatedAt)

	if errors.Is(err, sql.ErrNoRows){
		return nil, nil
	}

    if err != nil {
        return nil, fmt.Errorf("OrderRepo.GetOrderByID: %w", err)
    }
}