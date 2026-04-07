"""
Kafka consumer for authorization and entity events.
Feeds events into the fraud detection pipeline.
"""

import json
import logging
import threading

from confluent_kafka import Consumer, KafkaError

from config import Config
from detector import FraudDetector
from kill_chain import KillChain

logger = logging.getLogger(__name__)


class EventConsumer:
    """Consumes events from Kafka/Redpanda and runs them through the fraud pipeline."""

    def __init__(self, detector: FraudDetector, kill_chain: KillChain, config: Config):
        self.detector = detector
        self.kill_chain = kill_chain
        self.config = config
        self._running = False
        self._thread: threading.Thread | None = None

        self.consumer = Consumer({
            "bootstrap.servers": config.KAFKA_BROKERS,
            "group.id": config.KAFKA_GROUP_ID,
            "auto.offset.reset": "latest",
            "enable.auto.commit": True,
        })

    def start(self):
        """Start consuming events in a background thread."""
        self._running = True
        self.consumer.subscribe(self.config.KAFKA_TOPICS)
        self._thread = threading.Thread(target=self._consume_loop, daemon=True)
        self._thread.start()
        logger.info(
            "Event consumer started, subscribed to: %s",
            ", ".join(self.config.KAFKA_TOPICS),
        )

    def stop(self):
        self._running = False
        if self._thread:
            self._thread.join(timeout=5)
        self.consumer.close()

    def _consume_loop(self):
        while self._running:
            msg = self.consumer.poll(timeout=1.0)
            if msg is None:
                continue
            if msg.error():
                if msg.error().code() == KafkaError._PARTITION_EOF:
                    continue
                logger.error("Kafka error: %s", msg.error())
                continue

            try:
                event = json.loads(msg.value().decode("utf-8"))
                self._process_event(event, msg.topic())
            except Exception as e:
                logger.error("Failed to process event: %s", e)

    def _process_event(self, event: dict, topic: str):
        """Run event through detect → classify → kill chain."""
        # Stage 1: Detect
        detection = self.detector.detect(event)
        if detection is None:
            return  # Normal event

        logger.warning(
            "ANOMALY DETECTED: entity=%s score=%.3f velocity=%d topic=%s",
            detection["entity_id"],
            detection["anomaly_score"],
            detection["velocity"],
            topic,
        )

        # Stage 2: Classify
        classification = self.detector.classify(detection)
        logger.warning(
            "CLASSIFIED: entity=%s type=%s severity=%s",
            classification["entity_id"],
            classification["attack_type"],
            classification["severity"],
        )

        # Stages 3-6: Kill chain
        incident = self.kill_chain.execute(classification)
        logger.info(
            "KILL CHAIN COMPLETE: incident=%s total_stages=%d",
            incident["id"],
            len(incident.get("timeline", [])),
        )
