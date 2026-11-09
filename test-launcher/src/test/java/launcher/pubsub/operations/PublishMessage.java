package launcher.pubsub.operations;

import com.google.api.client.util.Strings;
import com.google.api.core.ApiFuture;
import com.google.protobuf.ByteString;
import com.google.pubsub.v1.PubsubMessage;
import com.google.pubsub.v1.PubsubMessage.Builder;
import io.cloudevents.CloudEvent;
import io.cloudevents.core.builder.CloudEventBuilder;
import launcher.pubsub.PubSub.PubSubData;

import java.net.URI;
import java.time.OffsetDateTime;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.TimeUnit;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class PublishMessage implements PubSubOperation {
    private static final Logger logger = LoggerFactory.getLogger(PublishMessage.class);
    private static final long PUBLISH_TIMEOUT_SECONDS = 30;

    /** Adds the CloudEvents ce-* attributes when both ceType and ceSource are given. */
    static void addCloudEventAttributes(final Builder builder, final PubSubData pubSubData) {
        if (Strings.isNullOrEmpty(pubSubData.ceType) || Strings.isNullOrEmpty(pubSubData.ceSource)) {
            return;
        }
        CloudEvent event = CloudEventBuilder.v1().withId(UUID.randomUUID().toString())
                .withType(pubSubData.ceType).withSource(URI.create(pubSubData.ceSource))
                .withTime(OffsetDateTime.now()).build();
        event.getAttributeNames().forEach(attribute -> builder.putAttributes("ce-" + attribute,
                Objects.requireNonNull(event.getAttribute(attribute)).toString()));
    }

    static PubsubMessage buildMessage(final PubSubData pubSubData) {
        Builder builder = PubsubMessage.newBuilder();
        addCloudEventAttributes(builder, pubSubData);
        if (pubSubData.attributes != null && !pubSubData.attributes.isEmpty()) {
            pubSubData.attributes.forEach(builder::putAttributes);
        }
        builder.putAttributes("Content-Type", "application/json");
        if (!Strings.isNullOrEmpty(pubSubData.orderingKey)) {
            builder.setOrderingKey(pubSubData.orderingKey);
        }
        return builder.setData(ByteString.copyFromUtf8(pubSubData.message)).build();
    }

    /** Publishes the message and waits for the server ack, so a failed publish is never reported as OK. */
    public String execute(PubSubData pubSubData) throws Exception {
        PubsubMessage pubsubMessage = buildMessage(pubSubData);
        ApiFuture<String> future = pubSubData.publisher.publish(pubsubMessage);
        String messageId = future.get(PUBLISH_TIMEOUT_SECONDS, TimeUnit.SECONDS);
        logger.info("Published message ID {}. Message: {} with orderingKey {}", messageId,
                pubsubMessage.getData().toStringUtf8(), pubSubData.orderingKey);
        return "OK";
    }
}
