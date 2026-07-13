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

import java.util.*;
import java.util.concurrent.CompletableFuture;

public class FindMessageAsync implements PubSubOperation {

    private static final Logger logger = LoggerFactory.getLogger(FindMessageAsync.class);
    public static final String OK = "OK";
    public static final String KO = "KO";

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
                        consumer.ack();
                        String stringMessage = message.getData().toStringUtf8();
                        JsonNode messageTree = Utils.mapperMessageToJsonNode(stringMessage, ignoreFieldsCopy);

                        logger.info("Received Message - OrderingKey: {}, Data: {}, Attributes: {}",
                                message.getOrderingKey(),
                                messageTree.toPrettyString(),
                                message.getAttributesMap());

                        if (Utils.compareJsonNodes(expectedMessageTree, messageTree)
                                && Utils.isValidOrdering(pubSubData, message)
                                && Utils.containsAttributes(pubSubData, message)) {
                            messageResult.complete(OK);
                        }
                    })
                    .setCredentialsProvider(NoCredentialsProvider.create())
                    .setChannelProvider(pubSubData.channelProvider).build();
            subscriber.startAsync().awaitRunning();

            Timer timer = new Timer();
            timer.schedule(new TimerTask() {
                @Override
                public void run() {
                    messageResult.obtrudeException(new RuntimeException("message not found after timeout"));
                }
            }, timeoutSeconds * 1000);

            return messageResult.get();
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
