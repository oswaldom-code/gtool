package launcher.pubsub.operations;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.google.pubsub.v1.AcknowledgeRequest;
import com.google.pubsub.v1.ProjectSubscriptionName;
import com.google.pubsub.v1.PullRequest;
import com.google.pubsub.v1.PullResponse;
import com.google.pubsub.v1.ReceivedMessage;
import launcher.pubsub.Utils;
import launcher.pubsub.PubSub.PubSubData;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Optional;
import java.util.stream.Collectors;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class ConsumeOrderedMessages implements PubSubOperation {

    private final Logger logger = LoggerFactory.getLogger(ConsumeOrderedMessages.class);

    public String execute(PubSubData pubSubData) throws JsonProcessingException {
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

        List<JsonNode> expectedMessagesTree = pubSubData.orderedMessages.stream()
                .map(m -> Utils.mapperMessageToJsonNode(m, pubSubData))
                .collect(Collectors.toList());

        List<JsonNode> receivedMessagesTree = new ArrayList<>();

        messages.stream()
                .forEach(message -> {
                    if (pubSubData.orderingKey.equals(message.getMessage().getOrderingKey())) {
                        logger.info("Group ordered message for acknowledgement is {}.", message.getMessage().getData().toStringUtf8());
                        AcknowledgeRequest acknowledgeRequest = AcknowledgeRequest.newBuilder()
                                .setSubscription(subscriptionName)
                                .addAckIds(message.getAckId()).build();
                        // pubSubData.subscriptionClient.acknowledgeCallable().call(acknowledgeRequest);
                        pubSubData.subscriptionClient.acknowledge(acknowledgeRequest);

                        receivedMessagesTree.add(
                                Utils.mapperMessageToJsonNode(message.getMessage().getData().toStringUtf8(), pubSubData));
                    } else {
                        // Nack message
                        pubSubData.subscriptionClient.modifyAckDeadline(subscriptionName,
                                Arrays.asList(message.getAckId()), 0);
                    }
                });

        if (expectedMessagesTree.size() != receivedMessagesTree.size()) {
            throw new RuntimeException(String.format("Number of messages is invalid. Expected: %d - Received: %d",
                    expectedMessagesTree.size(), receivedMessagesTree.size()));
        }

        boolean isOk = true;
        for (int i = 0; i < expectedMessagesTree.size(); i++) {
            if (!expectedMessagesTree.get(i).equals(receivedMessagesTree.get(i))) {
                isOk = false;
                break;
            }
        }

        if (isOk) {
            logger.info("{} messages consumed correctly", expectedMessagesTree.size());
        } else {
            throw new RuntimeException("Received messages have a different order than expected ones");
        }

        return "OK";
    }

}
