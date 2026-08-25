package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrProductNotFound = errors.New("product Not Found")
var ErrInsufficientStock = errors.New("insufficient Stock")

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
	ProductID int64 `json:"product_id" binding:"required"`
	Quantity  int   `json:"quantity" binding:"required,gte=1,lte=100"`
}

func (o *OrderRepo) CreateOrder(userID int64, items []OrderInput) (*Order, error){

	orderItems := make([]OrderItem, 0, len(items))
	var totalCents int64 

	//建立Transaction，只有全部DB操作成功才套用，其中一步失敗就整個操作不算。
	tx, err := o.db.Begin()
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
		`, input.ProductID).Scan(&productName, &priceCents, &stock)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: productID = %d", ErrProductNotFound, input.ProductID)
		}

		if err != nil {
			return nil, fmt.Errorf("OrderRepo.CreateOrder: query product %w", err)
		}

		if int64(input.Quantity) > stock {
			return nil, fmt.Errorf("%w: productID = %d", ErrInsufficientStock, input.ProductID)
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

	err = tx.QueryRow(`
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
			RETURNING id
		`, orderItems[i].OrderID, orderItems[i].ProductID, orderItems[i].Quantity, orderItems[i].UnitPriceCents).Scan(&id)

		if err != nil {
            return nil, fmt.Errorf("OrderRepo.CreateOrder: insert item: %w", err)
        }
		orderItems[i].ID = id

		if _, err := tx.Exec(`
			UPDATE products SET stock = stock - $1 WHERE id = $2
		`, orderItems[i].Quantity, orderItems[i].ProductID); err != nil {
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
		SELECT id, user_id, total_cents, status, created_at
		FROM orders WHERE id = $1 AND user_id = $2
	`, orderID, userID).Scan(&order.ID, &order.UserID, &order.TotalCents, &order.Status, &order.CreatedAt)

	if errors.Is(err, sql.ErrNoRows){
		return nil, nil
	}

    if err != nil {
        return nil, fmt.Errorf("OrderRepo.GetOrderByID: %w", err)
    }

	rows, err := o.db.Query(`
		SELECT oi.id, oi.order_id, oi.product_id, pd.name, oi.quantity, oi.unit_price_cents
		FROM order_items oi JOIN products pd ON oi.product_id = pd.id
		WHERE oi.order_id = $1 ORDER BY oi.id
	`, orderID)

	if err != nil {
		return nil, fmt.Errorf("OrderRepo.GetOrderByID: query product items %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var oi OrderItem
		err := rows.Scan(&oi.ID, &oi.OrderID, &oi.ProductID, &oi.ProductName, &oi.Quantity, &oi.UnitPriceCents)
		if err != nil {
			return nil, fmt.Errorf("OrderRepo.GetOrderByID: scan product %w", err)
		}
		order.Items = append(order.Items, oi)
	}

	//可能連線斷了造成錯誤
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("OrderRepo.GetOrderByID: %w", err)
	}

	return &order, nil
}

func (o *OrderRepo) ListOrderByUserID(userID int64) ([]Order, error) {
	rows, err := o.db.Query(`
		SELECT id, user_id, total_cents, status, created_at
		FROM orders WHERE user_id = $1 ORDER BY id DESC
	`, userID)

	if err != nil {
		return nil, fmt.Errorf("OrderRepo.ListOrderByUserID: %w", err)
	}

	orders := []Order{}
	defer rows.Close()
	for rows.Next() {
		var o Order
		err := rows.Scan(&o.ID, &o.UserID, &o.TotalCents, &o.Status, &o.CreatedAt)

		if err != nil {
			return nil, fmt.Errorf("OrderRepo.ListOrderByUserID: Scan %w", err)
		}
		orders = append(orders, o)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("OrderRepo.ListOrderByUserID:  %w", err)
	}

	return orders, nil
}

//假設現在要列出使用者的訂單，同時列出每個訂單的商品詳細資訊，需要打資料庫連線 N + 1 次，會需要等非常久
//解決: 用Join撈出
func (r *OrderRepo) ListOrdersWithItemsByUserID(userID int64) ([]Order, error) {
    query := `
        SELECT
            o.id, o.user_id, o.total_cents, o.status, o.created_at,
            oi.id, oi.product_id, oi.quantity, oi.unit_price_cents, p.name
        FROM orders o
        LEFT JOIN order_items oi ON oi.order_id = o.id
        LEFT JOIN products p ON p.id = oi.product_id
        WHERE o.user_id = $1
        ORDER BY o.id DESC, oi.id
    `
    rows, err := r.db.Query(query, userID)
    if err != nil {
        return nil, fmt.Errorf("ListOrdersWithItems: %w", err)
    }
    defer rows.Close()

    // 用 map 分組（key = orderID，value = *Order）
    // 因為要改變 Order 的 Items 屬性，所以需要用 Pointer
    orderMap := make(map[int64]*Order)
    var orderList []*Order  // 保持順序

    for rows.Next() {
        var oID int64
        var userID int64
        var totalCents int64
        var status string
        var createdAt time.Time

        // items 欄位可能是 NULL（LEFT JOIN 訂單沒 items 時）
        var itemID sql.NullInt64
        var productID sql.NullInt64
        var quantity sql.NullInt32
        var unitPrice sql.NullInt64
        var productName sql.NullString

        err := rows.Scan(
            &oID, &userID, &totalCents, &status, &createdAt,
            &itemID, &productID, &quantity, &unitPrice, &productName,
        )
        if err != nil {
            return nil, fmt.Errorf("ListOrdersWithItems: scan: %w", err)
        }

        // 檢查 map 有沒有這筆 order
        order, exists := orderMap[oID]
        if !exists {
            order = &Order{
                ID:         oID,
                UserID:     userID,
                TotalCents: totalCents,
                Status:     status,
                CreatedAt:  createdAt,
                Items:      []OrderItem{},
            }
            orderMap[oID] = order
            orderList = append(orderList, order)
        }

        // 如果有 item，加進 order
        if itemID.Valid {
            order.Items = append(order.Items, OrderItem{
                ID:             itemID.Int64,
                OrderID:        oID,
                ProductID:      productID.Int64,
                ProductName:    productName.String,
                Quantity:       int(quantity.Int32),
                UnitPriceCents: unitPrice.Int64,
            })
        }
    }
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("ListOrdersWithItems: %w", err)
    }

    // *Order → Order
    result := make([]Order, 0, len(orderList))
    for _, o := range orderList {
        result = append(result, *o)
    }
    return result, nil
}
