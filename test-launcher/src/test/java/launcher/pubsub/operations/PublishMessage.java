package launcher.pubsub.operations;

import com.google.api.client.util.Strings;
import com.google.api.core.ApiFuture;
import com.google.api.core.ApiFutureCallback;
import com.google.api.core.ApiFutures;
import com.google.api.gax.rpc.ApiException;
import com.google.common.util.concurrent.MoreExecutors;
import com.google.protobuf.ByteString;
import com.google.pubsub.v1.PubsubMessage;
import com.google.pubsub.v1.PubsubMessage.Builder;
import io.cloudevents.CloudEvent;
import io.cloudevents.core.builder.CloudEventBuilder;
import launcher.pubsub.PubSub.PubSubData;

import java.io.IOException;
import java.net.URI;
import java.time.OffsetDateTime;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.TimeUnit;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class PublishMessage implements PubSubOperation {
    private final Logger logger = LoggerFactory.getLogger(PublishMessage.class);

    @SuppressWarnings("null")
    public String execute(PubSubData pubSubData) throws IOException {
        try {
            CloudEvent event = CloudEventBuilder.v1().withId(UUID.randomUUID().toString())
                    .withType(pubSubData.ceType).withSource(URI.create(pubSubData.ceSource))
                    .withTime(OffsetDateTime.now()).build();

            ByteString data = ByteString.copyFromUtf8(pubSubData.message);
            Builder pubsubMessageBuilder = PubsubMessage.newBuilder();

            event.getAttributeNames().forEach(attribute -> pubsubMessageBuilder.putAttributes("ce-" + attribute,
                    Objects.requireNonNull(event.getAttribute(attribute)).toString()));

            if (pubSubData.attributes != null && !pubSubData.attributes.isEmpty()) {
                pubSubData.attributes.forEach(pubsubMessageBuilder::putAttributes);
            }

            pubsubMessageBuilder.putAttributes("Content-Type", "application/json");

            if (!Strings.isNullOrEmpty(pubSubData.orderingKey)) {
                pubsubMessageBuilder.setOrderingKey(pubSubData.orderingKey);
            }

            PubsubMessage pubsubMessage = pubsubMessageBuilder.setData(data).build();

            ApiFuture<String> future = pubSubData.publisher.publish(pubsubMessage);

            // Add an asynchronous callback to handle success / failure
            ApiFutures.addCallback(future, new ApiFutureCallback<String>() {

                @Override
                public void onFailure(Throwable throwable) {
                    if (throwable instanceof ApiException) {
                        ApiException apiException = ((ApiException) throwable);
                        // details on the API exception
                        logger.error("onFailure. CODE: {}. Retryable: {}", apiException.getStatusCode().getCode(), apiException.isRetryable());
                    }
                    logger.error("Error publishing message : {}.", pubSubData.message, throwable);
                }

                @Override
                public void onSuccess(String messageId) {
                    // Once published, returns server-assigned message ids (unique within the topic)
                    logger.info("Published message ID {}. Message: {} with orderingKey {}", messageId, data.toStringUtf8(), pubSubData.orderingKey);
                }
            }, MoreExecutors.directExecutor());

        } finally {
            if (pubSubData.publisher != null) {
                // When finished with the publisher, shutdown to free up resources.
                pubSubData.publisher.shutdown();
                try {
                    pubSubData.publisher.awaitTermination(1, TimeUnit.MINUTES);
                } catch (InterruptedException e) {
                    logger.error("Exception shutting down pubsub ", e);
                }
            }
        }
        return "OK";
    }
}
