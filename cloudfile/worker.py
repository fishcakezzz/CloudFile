import argparse
import time

from sqlalchemy import select

from cloudfile.config import get_settings
from cloudfile.database import SessionLocal, init_db
from cloudfile.dependencies import get_media_processor, get_media_queue, get_storage
from cloudfile.models import MediaTask, MediaTaskStatus
from cloudfile.queue import MediaTaskMessage, QueueAction
from cloudfile.services import run_media_task


def handle_media_message(message: MediaTaskMessage) -> QueueAction:
    with SessionLocal() as db:
        _, action = run_media_task(db, get_storage(), get_media_processor(), message.task_id)
        return action


def consume_queue() -> None:
    init_db()
    get_media_queue().consume_media_task(handle_media_message)


def poll_inline() -> None:
    init_db()
    queue = get_media_queue()
    while True:
        queue.consume_media_task(handle_media_message)
        with SessionLocal() as db:
            task = db.scalar(select(MediaTask).where(MediaTask.status == MediaTaskStatus.PENDING).order_by(MediaTask.id))
            if task:
                action = handle_media_message(MediaTaskMessage(task_id=task.id))
                if action == QueueAction.RETRY:
                    queue.publish_retry(MediaTaskMessage(task_id=task.id))
                elif action == QueueAction.DEAD:
                    queue.publish_dead(MediaTaskMessage(task_id=task.id))
        time.sleep(2)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=["auto", "consume", "poll"], default="auto")
    args = parser.parse_args()
    settings = get_settings()
    if args.mode == "consume" or (args.mode == "auto" and settings.queue_driver.lower() == "rabbitmq"):
        consume_queue()
    else:
        poll_inline()


if __name__ == "__main__":
    main()
