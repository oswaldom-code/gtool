package launcher.pubsub.operations;

import com.fasterxml.jackson.databind.JsonNode;
import com.google.api.gax.core.NoCredentialsProvider;
import com.google.api.gax.grpc.GrpcTransportChannel;
import com.google.api.gax.rpc.FixedTransportChannelProvider;
import com.google.cloud.pubsub.v1.AckReplyConsumer;
import com.google.cloud.pubsub.v1.MessageReceiver;
import com.google.cloud.pubsub.v1.Subscriber;
import com.google.pubsub.v1.AcknowledgeRequest;
import com.google.pubsub.v1.ProjectSubscriptionName;
import com.google.pubsub.v1.PubsubMessage;
import com.google.pubsub.v1.PullRequest;
import com.google.pubsub.v1.PullResponse;
import com.google.pubsub.v1.ReceivedMessage;
import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import launcher.pubsub.Utils;
import launcher.pubsub.PubSub.PubSubData;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Collections;
import java.util.List;
import java.util.Objects;
import java.util.Optional;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

import static org.awaitility.Awaitility.await;

public class ConsumeMessage implements PubSubOperation {

    private static final Logger logger = LoggerFactory.getLogger(ConsumeMessage.class);

    public String execute(PubSubData pubSubData) {
        if (Objects.nonNull(pubSubData.timeoutConsumeMessageSeconds)) {
            return executeWithTimeout(pubSubData);
        } else {
            return executeWithoutTimeout(pubSubData);
        }
    }

    private String executeWithTimeout(PubSubData pubSubData) {

        AtomicBoolean messageFound = new AtomicBoolean(false);
        ProjectSubscriptionName subscriptionName =
                ProjectSubscriptionName.of(pubSubData.projectId, pubSubData.subscriptionId);

        JsonNode expectedMessageTree = Utils.mapperMessageToJsonNode(pubSubData.expectedMessage, pubSubData);

        String hostPort = System.getenv("PUBSUB_EMULATOR_HOST");
        ManagedChannel channel = ManagedChannelBuilder.forTarget(hostPort).usePlaintext().build();
        FixedTransportChannelProvider channelProvider = FixedTransportChannelProvider.create(GrpcTransportChannel.create(channel));

        MessageReceiver receiver =
                (PubsubMessage message, AckReplyConsumer consumer) -> {
                    JsonNode messageTree = Utils.mapperMessageToJsonNode(message.getData().toStringUtf8(),
                            pubSubData);
                    if (Utils.compareJsonNodes(expectedMessageTree, messageTree) &&
                        Utils.isValidOrdering(pubSubData, message) &&
                        Utils.containsAttributes(pubSubData, message)) {
                        consumer.ack();
                        messageFound.set(true);
                    }
                };

        Subscriber subscriber = null;
        try {
            subscriber = Subscriber.newBuilder(subscriptionName, receiver)
                    .setCredentialsProvider(NoCredentialsProvider.create())
                    .setChannelProvider(channelProvider).build();
            subscriber.startAsync();

            await().atMost(pubSubData.timeoutConsumeMessageSeconds, TimeUnit.SECONDS)
                    .until(messageFound::get);

        } catch (Exception e) {
            logger.error("error consuming messages", e);
        } finally {
            logger.info("finally consuming messages, stopping subscriber");
            Objects.requireNonNull(subscriber).stopAsync();
        }

        if (!messageFound.get()) {
            throw new RuntimeException("message to consume not found");
        }

        logger.info("message to consume found successfully");
        return "OK";
    }

    private String executeWithoutTimeout(PubSubData pubSubData) {
        logger.info("executing ConsumeMessage without timeout");
        String subscriptionName = ProjectSubscriptionName.format(pubSubData.projectId, pubSubData.subscriptionId);
        int numberMessages = Optional.ofNullable(pubSubData.maxRequestedMessages).orElse(MAX_REQUESTED_MESSAGES);
        PullRequest pullRequest = PullRequest.newBuilder().setMaxMessages(numberMessages)
                .setSubscription(subscriptionName)
                .build();

        // Use pullCallable().futureCall to asynchronously perform this operation.
        PullResponse pullResponse = pubSubData.subscriptionClient.pullCallable().call(pullRequest);
        List<ReceivedMessage> messages = pullResponse.getReceivedMessagesList();

        if (messages.isEmpty()) {
            throw new RuntimeException("No message to consume");
        }

        JsonNode expectedMessageTree = Utils.mapperMessageToJsonNode(pubSubData.expectedMessage, pubSubData);

        boolean notFoundMessage = true;
        for (ReceivedMessage message : messages) {
            JsonNode messageTree = Utils.mapperMessageToJsonNode(message.getMessage().getData().toStringUtf8(),
                    pubSubData);

            // Handle received message
            if (notFoundMessage && Utils.compareJsonNodes(expectedMessageTree, messageTree)
                    && Utils.isValidOrdering(pubSubData, message)
                    && Utils.containsAttributes(pubSubData, message)) {
                notFoundMessage = false;
                // Acknowledge received messages.
                AcknowledgeRequest acknowledgeRequest = AcknowledgeRequest.newBuilder()
                        .setSubscription(subscriptionName)
                        .addAckIds(message.getAckId()).build();

                // Use acknowledgeCallable().futureCall to asynchronously perform this
                // operation.
                pubSubData.subscriptionClient.acknowledgeCallable().call(acknowledgeRequest);
                logger.info("message: {} consumed correctly", message);
            } else {
                // Nack message
                pubSubData.subscriptionClient.modifyAckDeadline(subscriptionName, Collections.singletonList(message.getAckId()),
                        0);
            }
        }
        if (notFoundMessage) {
            throw new RuntimeException("Message to consume not found");
        }

        return "OK";
    }

}
