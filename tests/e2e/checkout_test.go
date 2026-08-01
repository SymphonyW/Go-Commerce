//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"go-commerce/internal/auth"
	"go-commerce/internal/idempotency"
	"go-commerce/internal/merchant"
	"go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/internal/payment"
	"go-commerce/internal/product"
)

type apiClient struct {
	baseURL string
	client  *http.Client
}

type authResponse struct {
	UserID int64  `json:"user_id"`
	Token  string `json:"token"`
	Role   string `json:"role"`
}

type merchantResponse struct {
	Merchant struct {
		ID int64 `json:"id"`
	} `json:"merchant"`
}

type addProductResponse struct {
	ProductID int64 `json:"product_id"`
}

type productListResponse struct {
	Products []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"products"`
}

type orderResponse struct {
	Order struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	} `json:"order"`
}

type paymentResponse struct {
	Payment struct {
		ID int64 `json:"id"`
	} `json:"payment"`
}

func TestCheckoutFlowThroughGateway(t *testing.T) {
	client := newAPIClient()
	waitForGatewayReady(t, client)

	suffix := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	merchantUsername := "merchant-" + suffix
	customerUsername := "customer-" + suffix
	productName := "e2e-product-" + suffix
	cleanupIDs := e2eCleanupIDs{}
	t.Cleanup(func() { cleanupE2EData(t, cleanupIDs) })

	merchantReg := client.register(t, merchantUsername, "merchant")
	cleanupIDs.merchantUserID = merchantReg.UserID
	merchantLogin := client.login(t, merchantUsername)
	if merchantLogin.Role != "merchant" {
		t.Fatalf("unexpected merchant role: got %q want merchant", merchantLogin.Role)
	}

	shopID := client.createMerchant(t, merchantLogin.Token)
	cleanupIDs.merchantID = shopID
	productID := client.addProduct(t, merchantLogin.Token, shopID, productName)
	cleanupIDs.productID = productID

	customerReg := client.register(t, customerUsername, "customer")
	cleanupIDs.customerUserID = customerReg.UserID
	customerLogin := client.login(t, customerUsername)
	if customerLogin.Role != "customer" {
		t.Fatalf("unexpected customer role: got %q want customer", customerLogin.Role)
	}

	client.assertProductVisible(t, productName, productID)
	client.addToCart(t, customerLogin.Token, productID)
	orderID := client.createOrder(t, customerLogin.Token, productID, "order-"+suffix)
	cleanupIDs.orderID = orderID
	paymentID := client.createPayment(t, customerLogin.Token, orderID)
	cleanupIDs.paymentID = paymentID
	client.markPaymentSucceeded(t, customerLogin.Token, paymentID, "payment-"+suffix)
	client.waitForOrderStatus(t, customerLogin.Token, orderID, "paid")

	detail := client.getOrder(t, customerLogin.Token, orderID)
	if got, want := detail.Order.Status, "paid"; got != want {
		t.Fatalf("unexpected final order status: got %q want %q", got, want)
	}

}

func newAPIClient() *apiClient {
	baseURL := os.Getenv("E2E_BASE_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}
	return &apiClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func waitForGatewayReady(t *testing.T, client *apiClient) {
	t.Helper()

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.client.Get(client.baseURL + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("gateway did not become ready at %s", client.baseURL)
}

func (c *apiClient) register(t *testing.T, username, role string) authResponse {
	t.Helper()
	var resp authResponse
	c.doJSON(t, http.MethodPost, "/api/register", "", map[string]string{
		"username": username,
		"password": "password123",
		"email":    username + "@example.com",
		"role":     role,
	}, nil, http.StatusOK, &resp)
	return resp
}

func (c *apiClient) login(t *testing.T, username string) authResponse {
	t.Helper()
	var resp authResponse
	c.doJSON(t, http.MethodPost, "/api/login", "", map[string]string{
		"username": username,
		"password": "password123",
	}, nil, http.StatusOK, &resp)
	return resp
}

func (c *apiClient) createMerchant(t *testing.T, token string) int64 {
	t.Helper()
	var resp merchantResponse
	c.doJSON(t, http.MethodPost, "/api/merchants", token, map[string]string{
		"name":         "E2E Shop",
		"contact_info": "e2e@example.com",
	}, nil, http.StatusOK, &resp)
	return resp.Merchant.ID
}

func (c *apiClient) addProduct(t *testing.T, token string, merchantID int64, name string) int64 {
	t.Helper()
	var resp addProductResponse
	c.doJSON(t, http.MethodPost, "/api/merchants/products", token, map[string]any{
		"merchant_id": merchantID,
		"name":        name,
		"description": "e2e product",
		"price_cents": 9990,
		"stock":       5,
		"category":    "e2e",
		"image_url":   "https://example.com/e2e.png",
	}, nil, http.StatusOK, &resp)
	return resp.ProductID
}

func (c *apiClient) assertProductVisible(t *testing.T, productName string, productID int64) {
	t.Helper()
	var resp productListResponse
	c.doJSON(t, http.MethodGet, "/api/products?keyword="+productName, "", nil, nil, http.StatusOK, &resp)
	for _, item := range resp.Products {
		if item.ID == productID && item.Name == productName {
			return
		}
	}
	t.Fatalf("product %d (%s) not found in product list", productID, productName)
}

func (c *apiClient) addToCart(t *testing.T, token string, productID int64) {
	t.Helper()
	c.doJSON(t, http.MethodPost, "/api/cart/items", token, map[string]any{
		"product_id": productID,
		"quantity":   1,
	}, nil, http.StatusOK, nil)
}

func (c *apiClient) createOrder(t *testing.T, token string, productID int64, idempotencyKey string) int64 {
	t.Helper()
	var resp orderResponse
	c.doJSON(t, http.MethodPost, "/api/orders", token, map[string]any{
		"items": []map[string]any{{"product_id": productID, "quantity": 1}},
	}, map[string]string{"Idempotency-Key": idempotencyKey}, http.StatusOK, &resp)
	return resp.Order.ID
}

func (c *apiClient) createPayment(t *testing.T, token string, orderID int64) int64 {
	t.Helper()
	var resp paymentResponse
	c.doJSON(t, http.MethodPost, "/api/payments", token, map[string]any{
		"order_id":       orderID,
		"payment_method": "mock_balance",
	}, nil, http.StatusOK, &resp)
	return resp.Payment.ID
}

func (c *apiClient) markPaymentSucceeded(t *testing.T, token string, paymentID int64, idempotencyKey string) {
	t.Helper()
	c.doJSON(t, http.MethodPost, fmt.Sprintf("/api/payments/%d/success", paymentID), token, nil, map[string]string{
		"Idempotency-Key": idempotencyKey,
	}, http.StatusOK, nil)
}

func (c *apiClient) waitForOrderStatus(t *testing.T, token string, orderID int64, expected string) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp := c.getOrder(t, token, orderID)
		if resp.Order.Status == expected {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("order %d did not reach status %q", orderID, expected)
}

func (c *apiClient) getOrder(t *testing.T, token string, orderID int64) orderResponse {
	t.Helper()
	var resp orderResponse
	c.doJSON(t, http.MethodGet, fmt.Sprintf("/api/orders/%d", orderID), token, nil, nil, http.StatusOK, &resp)
	return resp
}

func (c *apiClient) doJSON(t *testing.T, method, path, token string, body any, headers map[string]string, wantStatus int, out any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if got := resp.StatusCode; got != wantStatus {
		t.Fatalf("unexpected status for %s %s: got %d want %d body=%s", method, path, got, wantStatus, string(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("failed to decode response body: %v body=%s", err, string(raw))
		}
	}
}

type e2eCleanupIDs struct {
	merchantUserID int64
	customerUserID int64
	merchantID     int64
	productID      int64
	orderID        int64
	paymentID      int64
}

func cleanupE2EData(t *testing.T, ids e2eCleanupIDs) {
	t.Helper()
	if ids.merchantUserID == 0 && ids.customerUserID == 0 && ids.merchantID == 0 && ids.productID == 0 && ids.orderID == 0 && ids.paymentID == 0 {
		return
	}

	dsn := os.Getenv("E2E_DB_DSN")
	if dsn == "" {
		dsn = "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open e2e cleanup database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx := db.WithContext(ctx)
	_ = tx.Where("aggregate_type = ? AND aggregate_id IN ?", "payment", []string{fmt.Sprintf("%d", ids.paymentID)}).Delete(&outbox.Event{}).Error
	_ = tx.Where("aggregate_type = ? AND aggregate_id IN ?", "order", []string{fmt.Sprintf("%d", ids.orderID)}).Delete(&outbox.Event{}).Error
	_ = tx.Where("user_id IN ?", []int64{ids.customerUserID, ids.merchantUserID}).Delete(&idempotency.Record{}).Error
	_ = tx.Delete(&payment.Payment{}, ids.paymentID).Error
	_ = tx.Where("order_id = ?", ids.orderID).Delete(&order.OrderItem{}).Error
	_ = tx.Delete(&order.Order{}, ids.orderID).Error
	_ = tx.Delete(&product.Product{}, ids.productID).Error
	_ = tx.Delete(&merchant.Merchant{}, ids.merchantID).Error
	_ = tx.Delete(&auth.User{}, []int64{ids.customerUserID, ids.merchantUserID}).Error
}
