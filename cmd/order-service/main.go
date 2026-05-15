// order-service 服务入口文件
// 负责启动订单服务，处理订单的创建、查询和取消
// 使用gRPC协议提供服务，并集成RabbitMQ进行消息传递
package main

import (
	"log"
	"net"
	"os"

	// RabbitMQ客户端：用于消息队列操作
	"github.com/streadway/amqp"
	// MySQL驱动：用于连接MySQL数据库
	"gorm.io/driver/mysql"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"
	// gRPC服务器：用于提供gRPC服务
	"google.golang.org/grpc"

	// 导入订单服务的业务逻辑
	"go-commerce/internal/order"
	// 导入订单服务的protobuf生成代码
	pb "go-commerce/api/order"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
)

// main 函数是order-service服务的入口点
// 负责初始化数据库连接、自动迁移表结构、连接RabbitMQ、启动gRPC服务器
func main() {
	// 从环境变量获取数据库连接字符串
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		// 默认值，用于本地开发
		dsn = "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
	}

	// 连接数据库
	// 使用GORM打开数据库连接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// 自动迁移数据库表结构
	// 会根据order.Order和order.OrderItem结构体自动创建或更新数据库表
	if err := db.AutoMigrate(&order.Order{}, &order.OrderItem{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 从环境变量获取RabbitMQ连接地址
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		// 默认值，用于本地开发
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	exchangeName := os.Getenv("EVENT_EXCHANGE")
	if exchangeName == "" {
		exchangeName = mq.DefaultExchangeName
	}

	paymentTimeout, err := order.ParseOrderPaymentTimeoutMinutes(os.Getenv("ORDER_PAYMENT_TIMEOUT_MINUTES"))
	if err != nil {
		log.Fatalf("invalid ORDER_PAYMENT_TIMEOUT_MINUTES: %v", err)
	}

	// 当前阶段采用弱一致策略：RabbitMQ 暂不可用时主交易链路继续运行，但事件会丢失并记录告警。
	var publisher mq.Publisher = mq.NopPublisher{}
	var timeoutScheduler order.TimeoutScheduler = order.NopTimeoutScheduler{}
	var paymentConsumerChannel *amqp.Channel
	var timeoutConsumerChannel *amqp.Channel
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Printf("rabbitmq_connection_failed exchange=%s error=%v", exchangeName, err)
	} else {
		defer conn.Close()

		ch, channelErr := conn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_channel_open_failed exchange=%s error=%v", exchangeName, channelErr)
		} else {
			defer ch.Close()
			publisher = mq.NewRabbitMQPublisher(ch, exchangeName)
		}

		paymentConsumerChannel, channelErr = conn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_payment_consumer_channel_open_failed exchange=%s error=%v", exchangeName, channelErr)
		} else {
			defer paymentConsumerChannel.Close()
		}

		timeoutSchedulerChannel, channelErr := conn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_timeout_scheduler_channel_open_failed error=%v", channelErr)
		} else {
			defer timeoutSchedulerChannel.Close()
			if err := declareOrderTimeoutTopology(timeoutSchedulerChannel); err != nil {
				log.Printf("rabbitmq_timeout_topology_declare_failed error=%v", err)
			} else {
				timeoutScheduler = order.NewRabbitMQTimeoutScheduler(timeoutSchedulerChannel, order.OrderTimeoutDelayExchange)
			}
		}

		timeoutConsumerChannel, channelErr = conn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_timeout_consumer_channel_open_failed error=%v", channelErr)
		} else {
			defer timeoutConsumerChannel.Close()
		}
	}

	// 监听TCP端口
	// 监听50053端口，用于gRPC服务
	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 创建gRPC服务器
	s := grpc.NewServer()

	// 注册订单服务
	// 将order.NewService(db, publisher)创建的服务实例注册到gRPC服务器
	// 传入数据库连接和事件发布器
	pb.RegisterOrderServiceServer(s, order.NewServiceWithTimeout(db, publisher, timeoutScheduler, paymentTimeout))

	if paymentConsumerChannel != nil {
		go consumePaymentSucceededEvents(paymentConsumerChannel, exchangeName, db, publisher)
	}
	if timeoutConsumerChannel != nil {
		go consumeOrderTimeoutEvents(timeoutConsumerChannel, db, publisher)
	}

	// 启动服务
	// 打印服务监听地址
	log.Printf("order service listening at %v", lis.Addr())
	// 启动gRPC服务器，开始接受请求
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func declareOrderTimeoutTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(order.OrderTimeoutDelayExchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(order.OrderTimeoutDLX, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	delayQueue, err := ch.QueueDeclare(
		order.OrderTimeoutDelayQueue,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange":    order.OrderTimeoutDLX,
			"x-dead-letter-routing-key": events.OrderTimeoutCheckType,
		},
	)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(delayQueue.Name, events.OrderTimeoutCheckType, order.OrderTimeoutDelayExchange, false, nil); err != nil {
		return err
	}

	cancelQueue, err := ch.QueueDeclare(order.OrderTimeoutCancelQueue, true, false, false, false, nil)
	if err != nil {
		return err
	}
	return ch.QueueBind(cancelQueue.Name, events.OrderTimeoutCheckType, order.OrderTimeoutDLX, false, nil)
}

func consumePaymentSucceededEvents(ch *amqp.Channel, exchangeName string, db *gorm.DB, publisher mq.Publisher) {
	if err := ch.ExchangeDeclare(exchangeName, "topic", true, false, false, false, nil); err != nil {
		log.Printf("rabbitmq_exchange_declare_failed exchange=%s error=%v", exchangeName, err)
		return
	}
	queue, err := ch.QueueDeclare("order.payment.succeeded", true, false, false, false, nil)
	if err != nil {
		log.Printf("rabbitmq_queue_declare_failed queue=order.payment.succeeded error=%v", err)
		return
	}
	if err := ch.QueueBind(queue.Name, events.PaymentSucceededType, exchangeName, false, nil); err != nil {
		log.Printf("rabbitmq_queue_bind_failed queue=%s routing_key=%s error=%v", queue.Name, events.PaymentSucceededType, err)
		return
	}
	deliveries, err := ch.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Printf("rabbitmq_consume_failed queue=%s error=%v", queue.Name, err)
		return
	}

	consumer := order.NewPaymentSucceededConsumer(db, publisher, log.Default())
	log.Printf("order_payment_consumer_started exchange=%s queue=%s routing_key=%s", exchangeName, queue.Name, events.PaymentSucceededType)
	for delivery := range deliveries {
		if err := consumer.HandleDelivery(delivery); err != nil {
			log.Printf("payment_event_handle_failed error=%v", err)
		}
	}
}

func consumeOrderTimeoutEvents(ch *amqp.Channel, db *gorm.DB, publisher mq.Publisher) {
	if err := declareOrderTimeoutTopology(ch); err != nil {
		log.Printf("rabbitmq_timeout_topology_declare_failed error=%v", err)
		return
	}
	deliveries, err := ch.Consume(order.OrderTimeoutCancelQueue, "", false, false, false, false, nil)
	if err != nil {
		log.Printf("rabbitmq_consume_failed queue=%s error=%v", order.OrderTimeoutCancelQueue, err)
		return
	}

	consumer := order.NewOrderTimeoutConsumer(db, publisher, log.Default())
	log.Printf(
		"order_timeout_consumer_started delay_exchange=%s dlx=%s queue=%s routing_key=%s",
		order.OrderTimeoutDelayExchange,
		order.OrderTimeoutDLX,
		order.OrderTimeoutCancelQueue,
		events.OrderTimeoutCheckType,
	)
	for delivery := range deliveries {
		if err := consumer.HandleDelivery(delivery); err != nil {
			log.Printf("order_timeout_event_handle_failed error=%v", err)
		}
	}
}
