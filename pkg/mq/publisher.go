package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/streadway/amqp"
)

const (
	DefaultExchangeName = "ecommerce.events"
	exchangeKind        = "topic"
)

// Publisher 抽象消息发布能力，让业务层不直接依赖 RabbitMQ SDK。
type Publisher interface {
	Publish(ctx context.Context, routingKey string, event interface{}) error
}

// Channel 描述 RabbitMQPublisher 真正需要的 AMQP 能力，便于在单元测试中替换。
type Channel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

type identifiedEvent interface {
	GetEventID() string
}

// RabbitMQPublisher 使用 topic exchange 将领域事件写入 RabbitMQ。
type RabbitMQPublisher struct {
	channel  Channel
	exchange string
	now      func() time.Time
}

// NewRabbitMQPublisher 创建 RabbitMQ 发布器；exchange 为空时使用默认事件交换机。
func NewRabbitMQPublisher(channel Channel, exchange string) *RabbitMQPublisher {
	if exchange == "" {
		exchange = DefaultExchangeName
	}

	return &RabbitMQPublisher{
		channel:  channel,
		exchange: exchange,
		now:      time.Now,
	}
}

// Publish 将事件序列化为 JSON，并写入持久化 topic exchange。
func (p *RabbitMQPublisher) Publish(ctx context.Context, routingKey string, event interface{}) error {
	if p == nil || p.channel == nil {
		return errors.New("rabbitmq publisher channel is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if err := p.channel.ExchangeDeclare(
		p.exchange,
		exchangeKind,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", p.exchange, err)
	}

	message := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    p.now().UTC(),
		Body:         body,
	}
	if identified, ok := event.(identifiedEvent); ok {
		message.MessageId = identified.GetEventID()
	}

	if err := p.channel.Publish(p.exchange, routingKey, false, false, message); err != nil {
		return fmt.Errorf("publish event %s: %w", routingKey, err)
	}

	return nil
}

// NopPublisher 用于消息组件暂不可用时保持主交易链路可运行，代价是当前事件会丢失。
type NopPublisher struct{}

func (NopPublisher) Publish(ctx context.Context, routingKey string, event interface{}) error {
	return nil
}
