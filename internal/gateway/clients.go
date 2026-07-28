package gateway

import (
	"fmt"

	"google.golang.org/grpc"

	pbAuth "go-commerce/api/auth"
	pbCart "go-commerce/api/cart"
	pbMerchant "go-commerce/api/merchant"
	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	pbProduct "go-commerce/api/product"
	"go-commerce/internal/gateway/handler"
	"go-commerce/pkg/healthcheck"
)

type ClientAddresses struct {
	Auth     string
	Product  string
	Order    string
	Payment  string
	Merchant string
	Cart     string
}

type ClientConnections struct {
	Auth     *grpc.ClientConn
	Product  *grpc.ClientConn
	Order    *grpc.ClientConn
	Payment  *grpc.ClientConn
	Merchant *grpc.ClientConn
	Cart     *grpc.ClientConn
}

func DialClients(addrs ClientAddresses, opts ...grpc.DialOption) (handler.Clients, *ClientConnections, error) {
	conns := &ClientConnections{}

	authConn, err := grpc.Dial(addrs.Auth, opts...)
	if err != nil {
		return handler.Clients{}, conns, fmt.Errorf("dial auth-service: %w", err)
	}
	conns.Auth = authConn

	productConn, err := grpc.Dial(addrs.Product, opts...)
	if err != nil {
		conns.Close()
		return handler.Clients{}, conns, fmt.Errorf("dial product-service: %w", err)
	}
	conns.Product = productConn

	orderConn, err := grpc.Dial(addrs.Order, opts...)
	if err != nil {
		conns.Close()
		return handler.Clients{}, conns, fmt.Errorf("dial order-service: %w", err)
	}
	conns.Order = orderConn

	paymentConn, err := grpc.Dial(addrs.Payment, opts...)
	if err != nil {
		conns.Close()
		return handler.Clients{}, conns, fmt.Errorf("dial payment-service: %w", err)
	}
	conns.Payment = paymentConn

	merchantConn, err := grpc.Dial(addrs.Merchant, opts...)
	if err != nil {
		conns.Close()
		return handler.Clients{}, conns, fmt.Errorf("dial merchant-service: %w", err)
	}
	conns.Merchant = merchantConn

	cartConn, err := grpc.Dial(addrs.Cart, opts...)
	if err != nil {
		conns.Close()
		return handler.Clients{}, conns, fmt.Errorf("dial cart-service: %w", err)
	}
	conns.Cart = cartConn

	return handler.Clients{
		Auth:     pbAuth.NewAuthServiceClient(authConn),
		Product:  pbProduct.NewProductServiceClient(productConn),
		Order:    pbOrder.NewOrderServiceClient(orderConn),
		Payment:  pbPayment.NewPaymentServiceClient(paymentConn),
		Merchant: pbMerchant.NewMerchantServiceClient(merchantConn),
		Cart:     pbCart.NewCartServiceClient(cartConn),
	}, conns, nil
}

func (c *ClientConnections) Close() {
	if c == nil {
		return
	}
	for _, conn := range []*grpc.ClientConn{c.Auth, c.Product, c.Order, c.Payment, c.Merchant, c.Cart} {
		if conn != nil {
			_ = conn.Close()
		}
	}
}

func (c *ClientConnections) HealthDependencies() []healthcheck.Dependency {
	if c == nil {
		return nil
	}
	return []healthcheck.Dependency{
		{Name: "auth-service", Check: healthcheck.GRPCHealth(c.Auth, "")},
		{Name: "product-service", Check: healthcheck.GRPCHealth(c.Product, "")},
		{Name: "order-service", Check: healthcheck.GRPCHealth(c.Order, "")},
		{Name: "payment-service", Check: healthcheck.GRPCHealth(c.Payment, "")},
		{Name: "merchant-service", Check: healthcheck.GRPCHealth(c.Merchant, "")},
		{Name: "cart-service", Check: healthcheck.GRPCHealth(c.Cart, "")},
	}
}
