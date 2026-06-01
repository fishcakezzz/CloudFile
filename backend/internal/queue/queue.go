package queue

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"cloudfile/backend/internal/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	MediaExchange     = "cloudfile.media.exchange"
	ProcessQueue      = "cloudfile.media.process.queue"
	RetryQueue        = "cloudfile.media.retry.queue"
	DeadQueue         = "cloudfile.media.dead.queue"
	ProcessRoutingKey = "media.process"
	RetryRoutingKey   = "media.retry"
	DeadRoutingKey    = "media.dead"
)

type Action string

const (
	Ack   Action = "ACK"
	Retry Action = "RETRY"
	Dead  Action = "DEAD"
)

type TaskQueue interface {
	PublishMediaTask(ctx context.Context, taskID uint) error
	PublishRetry(ctx context.Context, taskID uint) error
	PublishDead(ctx context.Context, taskID uint) error
	ConsumeMediaTask(ctx context.Context, handler func(context.Context, uint) (Action, error)) error
}

func New(cfg config.Config) (TaskQueue, error) {
	if cfg.QueueDriver == "rabbitmq" {
		return NewRabbitMQQueue(cfg.RabbitMQURL, cfg.RabbitRetryTTL)
	}
	return NewInlineQueue(), nil
}

type InlineQueue struct {
	ch chan uint
}

func NewInlineQueue() *InlineQueue {
	return &InlineQueue{ch: make(chan uint, 1024)}
}

func (q *InlineQueue) PublishMediaTask(ctx context.Context, taskID uint) error {
	select {
	case q.ch <- taskID:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *InlineQueue) PublishRetry(ctx context.Context, taskID uint) error {
	return q.PublishMediaTask(ctx, taskID)
}

func (q *InlineQueue) PublishDead(ctx context.Context, taskID uint) error {
	return nil
}

func (q *InlineQueue) ConsumeMediaTask(ctx context.Context, handler func(context.Context, uint) (Action, error)) error {
	for {
		select {
		case taskID := <-q.ch:
			_, _ = handler(ctx, taskID)
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
}

type RabbitMQQueue struct {
	url      string
	retryTTL time.Duration
}

func NewRabbitMQQueue(url string, retryTTL time.Duration) (*RabbitMQQueue, error) {
	q := &RabbitMQQueue{url: url, retryTTL: retryTTL}
	conn, ch, err := q.channel()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer ch.Close()
	return q, q.declare(ch)
}

func (q *RabbitMQQueue) channel() (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(q.url)
	if err != nil {
		return nil, nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, ch, nil
}

func (q *RabbitMQQueue) declare(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(MediaExchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(ProcessQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(ProcessQueue, ProcessRoutingKey, MediaExchange, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(RetryQueue, true, false, false, false, amqp.Table{
		"x-message-ttl":             int32(q.retryTTL / time.Millisecond),
		"x-dead-letter-exchange":    MediaExchange,
		"x-dead-letter-routing-key": ProcessRoutingKey,
	}); err != nil {
		return err
	}
	if err := ch.QueueBind(RetryQueue, RetryRoutingKey, MediaExchange, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(DeadQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return ch.QueueBind(DeadQueue, DeadRoutingKey, MediaExchange, false, nil)
}

func (q *RabbitMQQueue) publish(ctx context.Context, routingKey string, taskID uint) error {
	conn, ch, err := q.channel()
	if err != nil {
		return err
	}
	defer conn.Close()
	defer ch.Close()
	if err := q.declare(ch); err != nil {
		return err
	}
	if err := ch.Confirm(false); err != nil {
		return err
	}
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	if err := ch.PublishWithContext(ctx, MediaExchange, routingKey, true, false, amqp.Publishing{
		ContentType:  "text/plain",
		DeliveryMode: amqp.Persistent,
		Body:         []byte(strconv.FormatUint(uint64(taskID), 10)),
	}); err != nil {
		return err
	}
	select {
	case confirmation := <-confirms:
		if !confirmation.Ack {
			return fmt.Errorf("rabbitmq publish was not confirmed")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *RabbitMQQueue) PublishMediaTask(ctx context.Context, taskID uint) error {
	return q.publish(ctx, ProcessRoutingKey, taskID)
}

func (q *RabbitMQQueue) PublishRetry(ctx context.Context, taskID uint) error {
	return q.publish(ctx, RetryRoutingKey, taskID)
}

func (q *RabbitMQQueue) PublishDead(ctx context.Context, taskID uint) error {
	return q.publish(ctx, DeadRoutingKey, taskID)
}

func (q *RabbitMQQueue) ConsumeMediaTask(ctx context.Context, handler func(context.Context, uint) (Action, error)) error {
	conn, ch, err := q.channel()
	if err != nil {
		return err
	}
	defer conn.Close()
	defer ch.Close()
	if err := q.declare(ch); err != nil {
		return err
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(ProcessQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case delivery := <-deliveries:
			taskID64, err := strconv.ParseUint(string(delivery.Body), 10, 64)
			if err != nil {
				_ = delivery.Ack(false)
				continue
			}
			action, err := handler(ctx, uint(taskID64))
			if err == nil {
				switch action {
				case Retry:
					err = q.PublishRetry(ctx, uint(taskID64))
				case Dead:
					err = q.PublishDead(ctx, uint(taskID64))
				}
			}
			if err != nil {
				_ = delivery.Nack(false, true)
				continue
			}
			_ = delivery.Ack(false)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
