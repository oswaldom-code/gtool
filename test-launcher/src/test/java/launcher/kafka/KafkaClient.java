package launcher.kafka;

import java.time.Duration;
import java.util.HashSet;
import java.util.List;
import java.util.Properties;
import java.util.Set;
import java.util.UUID;

import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.clients.consumer.KafkaConsumer;
import org.apache.kafka.clients.producer.KafkaProducer;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.apache.kafka.common.serialization.StringSerializer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import launcher.pubsub.Utils;

/**
 * Minimal Kafka helper. Publishes string messages to a topic on the local broker
 * and consumes them back to assert on their content. Returns "OK"/"KO". Exposed
 * to Karate via Java.type.
 */
public class KafkaClient {

    private static final String KO = "KO";
    private static final String OK = "OK";
    private static final String BOOTSTRAP_SERVERS = "localhost:9092";
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

    /**
     * Reads the topic from the beginning until a message matches expectedMessage
     * or the timeout expires. A fresh consumer group is used on every call so
     * messages published before the test started are seen.
     */
    public String consumeMessage(final String topic, final String expectedMessage,
                                 final List<String> ignoreFields, final Integer timeoutSeconds) {
        final Properties properties = new Properties();
        properties.setProperty(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, BOOTSTRAP_SERVERS);
        properties.setProperty(ConsumerConfig.GROUP_ID_CONFIG, "gtool-launcher-" + UUID.randomUUID());
        properties.setProperty(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "earliest");
        properties.setProperty(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class.getName());
        properties.setProperty(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class.getName());

        final Set<String> ignored = ignoreFields == null ? Set.of() : new HashSet<>(ignoreFields);
        final int timeout = timeoutSeconds == null ? DEFAULT_TIMEOUT_SECONDS : timeoutSeconds;
        final long deadline = System.currentTimeMillis() + timeout * 1000L;

        try (KafkaConsumer<String, String> consumer = new KafkaConsumer<>(properties)) {
            consumer.subscribe(List.of(topic));
            while (System.currentTimeMillis() < deadline) {
                for (ConsumerRecord<String, String> record : consumer.poll(Duration.ofMillis(500))) {
                    if (matches(expectedMessage, record.value(), ignored)) {
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
}
