from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from enum import StrEnum

import pika


MEDIA_EXCHANGE = "cloudfile.media.exchange"
PROCESS_QUEUE = "cloudfile.media.process.queue"
RETRY_QUEUE = "cloudfile.media.retry.queue"
DEAD_QUEUE = "cloudfile.media.dead.queue"
PROCESS_ROUTING_KEY = "media.process"
RETRY_ROUTING_KEY = "media.retry"
DEAD_ROUTING_KEY = "media.dead"


@dataclass(frozen=True)
class MediaTaskMessage:
    task_id: int


class QueueAction(StrEnum):
    ACK = "ACK"
    RETRY = "RETRY"
    DEAD = "DEAD"


class TaskQueue:
    def publish_media_task(self, message: MediaTaskMessage) -> None:
        raise NotImplementedError

    def consume_media_task(self, handler: Callable[[MediaTaskMessage], QueueAction | None]) -> None:
        raise NotImplementedError

    def publish_retry(self, message: MediaTaskMessage) -> None:
        self.publish_media_task(message)

    def publish_dead(self, message: MediaTaskMessage) -> None:
        self.publish_media_task(message)

    def publish(self, message: MediaTaskMessage) -> None:
        self.publish_media_task(message)


class InlineQueue(TaskQueue):
    def __init__(self) -> None:
        self.messages: list[MediaTaskMessage] = []
        self.retry_messages: list[MediaTaskMessage] = []
        self.dead_messages: list[MediaTaskMessage] = []

    def publish_media_task(self, message: MediaTaskMessage) -> None:
        self.messages.append(message)

    def publish_retry(self, message: MediaTaskMessage) -> None:
        self.retry_messages.append(message)
        self.messages.append(message)

    def publish_dead(self, message: MediaTaskMessage) -> None:
        self.dead_messages.append(message)

    def consume_media_task(self, handler: Callable[[MediaTaskMessage], QueueAction | None]) -> None:
        while self.messages:
            message = self.messages.pop(0)
            action = handler(message) or QueueAction.ACK
            if action == QueueAction.RETRY:
                self.publish_retry(message)
            elif action == QueueAction.DEAD:
                self.publish_dead(message)


class RabbitMQQueue(TaskQueue):
    def __init__(self, url: str, retry_ttl_ms: int = 30_000) -> None:
        self.url = url
        self.retry_ttl_ms = retry_ttl_ms
        self._setup()

    def _connect(self):
        return pika.BlockingConnection(pika.URLParameters(self.url))

    def _setup_channel(self, channel) -> None:
        channel.exchange_declare(exchange=MEDIA_EXCHANGE, exchange_type="direct", durable=True)
        channel.queue_declare(queue=PROCESS_QUEUE, durable=True)
        channel.queue_bind(queue=PROCESS_QUEUE, exchange=MEDIA_EXCHANGE, routing_key=PROCESS_ROUTING_KEY)
        channel.queue_declare(
            queue=RETRY_QUEUE,
            durable=True,
            arguments={
                "x-message-ttl": self.retry_ttl_ms,
                "x-dead-letter-exchange": MEDIA_EXCHANGE,
                "x-dead-letter-routing-key": PROCESS_ROUTING_KEY,
            },
        )
        channel.queue_bind(queue=RETRY_QUEUE, exchange=MEDIA_EXCHANGE, routing_key=RETRY_ROUTING_KEY)
        channel.queue_declare(queue=DEAD_QUEUE, durable=True)
        channel.queue_bind(queue=DEAD_QUEUE, exchange=MEDIA_EXCHANGE, routing_key=DEAD_ROUTING_KEY)

    def _setup(self) -> None:
        connection = self._connect()
        try:
            channel = connection.channel()
            self._setup_channel(channel)
        finally:
            connection.close()

    def _publish(self, routing_key: str, message: MediaTaskMessage) -> None:
        connection = self._connect()
        try:
            channel = connection.channel()
            self._setup_channel(channel)
            channel.confirm_delivery()
            channel.basic_publish(
                exchange=MEDIA_EXCHANGE,
                routing_key=routing_key,
                body=str(message.task_id).encode(),
                mandatory=True,
                properties=pika.BasicProperties(delivery_mode=pika.DeliveryMode.Persistent),
            )
        finally:
            connection.close()

    def publish_media_task(self, message: MediaTaskMessage) -> None:
        self._publish(PROCESS_ROUTING_KEY, message)

    def publish_retry(self, message: MediaTaskMessage) -> None:
        self._publish(RETRY_ROUTING_KEY, message)

    def publish_dead(self, message: MediaTaskMessage) -> None:
        self._publish(DEAD_ROUTING_KEY, message)

    def consume_media_task(self, handler: Callable[[MediaTaskMessage], QueueAction | None]) -> None:
        connection = self._connect()
        channel = connection.channel()
        self._setup_channel(channel)
        channel.basic_qos(prefetch_count=1)

        def callback(ch, method, properties, body):
            message = MediaTaskMessage(task_id=int(body.decode()))
            action = handler(message) or QueueAction.ACK
            if action == QueueAction.RETRY:
                self.publish_retry(message)
            elif action == QueueAction.DEAD:
                self.publish_dead(message)
            ch.basic_ack(delivery_tag=method.delivery_tag)

        channel.basic_consume(queue=PROCESS_QUEUE, on_message_callback=callback, auto_ack=False)
        channel.start_consuming()


MediaQueue = TaskQueue
InlineMediaQueue = InlineQueue
RabbitMQMediaQueue = RabbitMQQueue
