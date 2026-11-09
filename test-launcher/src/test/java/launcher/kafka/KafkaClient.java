package launcher.kafka;

import java.time.Duration;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.clients.consumer.KafkaConsumer;
import org.apache.kafka.clients.producer.KafkaProducer;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.common.PartitionInfo;
import org.apache.kafka.common.TopicPartition;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.apache.kafka.common.serialization.StringSerializer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import launcher.pubsub.Utils;
import launcher.util.RunPorts;

/**
 * Minimal Kafka helper. Publishes string messages to a topic on the local broker
 * and consumes them back to assert on their content. Returns "OK"/"KO". Exposed
 * to Karate via Java.type.
 */
public class KafkaClient {

    private static final String KO = "KO";
    private static final String OK = "OK";
    private static final String BOOTSTRAP_SERVERS = "localhost:" + RunPorts.port("kafka", "9092");
    private static final int DEFAULT_TIMEOUT_SECONDS = 10;
    private static final Logger logger = LoggerFactory.getLogger(KafkaClient.class);

    public String publishMessage(final String topic, final String messageText) {
        final Properties properties = new Properties();
        properties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, BOOTSTRAP_SERVERS);
        properties.setProperty(ProducerConfig.KEY_SERIALIZER_CLASS_CONFIG, StringSerializer.class.getName());
        properties.setProperty(ProducerConfig.VALUE_SERIALIZER_CLASS_CONFIG, StringSerializer.class.getName());

        try (KafkaProducer<String, String> producer = new KafkaProducer<>(properties)) {
            producer.send(new ProducerRecord<>(topic, messageText)).get();
            logger.info("message sent to kafka successfully");
            return OK;
        } catch (Exception e) {
            logger.error("error sending kafka message", e);
            return KO;
        }
    }

    /**
     * Reports whether actual matches expected as JSON, ignoring key order and the
     * given fields. Non-JSON payloads never match.
     */
    static boolean matches(final String expected, final String actual, final Set<String> ignoreFields) {
        try {
            return Utils.compareJsonNodes(
                    Utils.mapperMessageToJsonNode(expected, ignoreFields),
                    Utils.mapperMessageToJsonNode(actual, ignoreFields));
        } catch (RuntimeException e) {
            return false;
        }
    }

    /** Offset after the last matched message of each partition, shared by every call in this run. */
    private static final Map<TopicPartition, Long> CONSUMED = new ConcurrentHashMap<>();

    static void markConsumed(final TopicPartition partition, final long offset) {
        CONSUMED.merge(partition, offset + 1, Math::max);
    }

    static Long consumedOffset(final TopicPartition partition) {
        return CONSUMED.get(partition);
    }

    /**
     * Reads the topic until a message matches expectedMessage or the timeout
     * expires. The first consume of a topic starts from the beginning (so
     * messages published before the step are seen); later ones start after
     * the last matched message, so an identical message consumed by an earlier
     * scenario never satisfies a new assertion. No consumer group is used:
     * partitions are assigned explicitly, which is safe for parallel scenarios.
     */
    public String consumeMessage(final String topic, final String expectedMessage,
                                 final List<String> ignoreFields, final Integer timeoutSeconds) {
        final Properties properties = new Properties();
        properties.setProperty(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, BOOTSTRAP_SERVERS);
        properties.setProperty(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, "false");
        properties.setProperty(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class.getName());
        properties.setProperty(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class.getName());

        final Set<String> ignored = ignoreFields == null ? Set.of() : new HashSet<>(ignoreFields);
        final int timeout = timeoutSeconds == null ? DEFAULT_TIMEOUT_SECONDS : timeoutSeconds;
        final long deadline = System.currentTimeMillis() + timeout * 1000L;

        try (KafkaConsumer<String, String> consumer = new KafkaConsumer<>(properties)) {
            final List<TopicPartition> partitions = waitForPartitions(consumer, topic, deadline);
            consumer.assign(partitions);
            consumer.seekToBeginning(partitions);
            for (final TopicPartition partition : partitions) {
                final Long next = consumedOffset(partition);
                if (next != null) {
                    consumer.seek(partition, next);
                }
            }
            while (System.currentTimeMillis() < deadline) {
                for (ConsumerRecord<String, String> record : consumer.poll(Duration.ofMillis(500))) {
                    if (matches(expectedMessage, record.value(), ignored)) {
                        markConsumed(new TopicPartition(record.topic(), record.partition()), record.offset());
                        logger.info("kafka message matched on topic {}", topic);
                        return OK;
                    }
                }
            }
            logger.error("no kafka message on topic {} matched {}", topic, expectedMessage);
            return KO;
        } catch (Exception e) {
            logger.error("error consuming kafka messages", e);
            return KO;
        }
    }

    private static List<TopicPartition> waitForPartitions(final KafkaConsumer<String, String> consumer,
                                                          final String topic, final long deadline) {
        while (true) {
            final List<PartitionInfo> infos = consumer.partitionsFor(topic, Duration.ofSeconds(2));
            if (infos != null && !infos.isEmpty()) {
                final List<TopicPartition> partitions = new ArrayList<>();
                for (final PartitionInfo info : infos) {
                    partitions.add(new TopicPartition(info.topic(), info.partition()));
                }
                return partitions;
            }
            if (System.currentTimeMillis() >= deadline) {
                throw new IllegalStateException("topic " + topic + " has no partitions");
            }
        }
    }
}
