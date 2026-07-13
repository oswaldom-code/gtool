package launcher.kafka;

import java.util.Properties;

import org.apache.kafka.clients.producer.KafkaProducer;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.common.serialization.StringSerializer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Minimal Kafka producer helper. Publishes string messages to a topic on the
 * local broker. Returns "OK"/"KO". Exposed to Karate via Java.type.
 */
public class KafkaClient {

    private static final String KO = "KO";
    private static final String OK = "OK";
    private static final String BOOTSTRAP_SERVERS = "localhost:9092";
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
}
