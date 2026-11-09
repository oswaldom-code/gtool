package launcher.pubsub.operations;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.google.api.gax.core.NoCredentialsProvider;
import com.google.cloud.pubsub.v1.AckReplyConsumer;
import com.google.cloud.pubsub.v1.Subscriber;
import com.google.pubsub.v1.ProjectSubscriptionName;
import com.google.pubsub.v1.PubsubMessage;

import launcher.pubsub.PubSub.PubSubData;
import launcher.pubsub.Utils;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.HashSet;
import java.util.Objects;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

public class FindMessageAsync implements PubSubOperation {

    private static final Logger logger = LoggerFactory.getLogger(FindMessageAsync.class);
    public static final String OK = "OK";
    public static final String KO = "KO";

    private static boolean matches(JsonNode expected, PubsubMessage message, Set<String> ignoreFields,
            PubSubData pubSubData) {
        try {
            JsonNode messageTree = Utils.mapperMessageToJsonNode(message.getData().toStringUtf8(), ignoreFields);
            logger.info("Received Message - OrderingKey: {}, Data: {}, Attributes: {}",
                    message.getOrderingKey(), messageTree.toPrettyString(), message.getAttributesMap());
            return Utils.compareJsonNodes(expected, messageTree)
                    && Utils.isValidOrdering(pubSubData, message)
                    && Utils.containsAttributes(pubSubData, message);
        } catch (RuntimeException e) {
            logger.warn("skipping message that is not valid JSON: {}", e.getMessage());
            return false;
        }
    }

    public String execute(PubSubData pubSubData) throws JsonProcessingException {

        long timeoutSeconds = 30;
        if (Objects.nonNull(pubSubData.timeoutConsumeMessageSeconds)) {
            timeoutSeconds = pubSubData.timeoutConsumeMessageSeconds;
        }

        ProjectSubscriptionName subscriptionName = ProjectSubscriptionName.of(pubSubData.projectId, pubSubData.subscriptionId);
        Subscriber subscriber = null;

        try {
            CompletableFuture<String> messageResult = new CompletableFuture<>();

            Set<String> ignoreFieldsCopy = new HashSet<>();
            if (pubSubData.ignoreMessageFields != null && !pubSubData.ignoreMessageFields.isEmpty()) {
                ignoreFieldsCopy.addAll(pubSubData.ignoreMessageFields);
            }

            JsonNode expectedMessageTree = Utils.mapperMessageToJsonNode(pubSubData.expectedMessage, ignoreFieldsCopy);

            subscriber = Subscriber.newBuilder(subscriptionName,
                (PubsubMessage message, AckReplyConsumer consumer) -> {
                        if (!messageResult.isDone() && matches(expectedMessageTree, message, ignoreFieldsCopy, pubSubData)) {
                            consumer.ack();
                            messageResult.complete(OK);
                        } else {
                            // Only the expected message is consumed; the rest stay
                            // available to later operations in the scenario.
                            consumer.nack();
                        }
                    })
                    .setCredentialsProvider(NoCredentialsProvider.create())
                    .setChannelProvider(pubSubData.channelProvider).build();
            subscriber.startAsync().awaitRunning();

            return messageResult.get(timeoutSeconds, TimeUnit.SECONDS);
        } catch (TimeoutException e) {
            logger.error("message not found after {}s", timeoutSeconds);
            return KO;
        } catch (Exception e) {
            logger.error("error searching message", e);
            return KO;
        } finally {
            if (subscriber != null) {
                subscriber.stopAsync().awaitTerminated();
            }
        }
    }
}
