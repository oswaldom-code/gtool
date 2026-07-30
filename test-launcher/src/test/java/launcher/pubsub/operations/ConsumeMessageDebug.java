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

import java.util.Arrays;
import java.util.List;
import java.util.Optional;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class ConsumeMessageDebug implements PubSubOperation {

    private final Logger logger = LoggerFactory.getLogger(ConsumeMessageDebug.class);

    public String execute(PubSubData pubSubData) throws JsonProcessingException {

        String log = "";
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
        log = log + expectedMessageTree.toPrettyString()  + "\n";

        boolean notFoundMessage = true;

        log = log + "****************************************************************" + "\n";

        for (ReceivedMessage message : messages) {
            JsonNode messageTree = Utils.mapperMessageToJsonNode(message.getMessage().getData().toStringUtf8(),
                    pubSubData);

            log = log + "New message was found! \n";
            log = log + messageTree.toPrettyString() + "\n";
            log = log + "************************" + expectedMessageTree.equals(messageTree) + "****************************************" + "\n";
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

                logger.info("Message: {} consumed correctly", message);
            } else {
                // Nack message
                pubSubData.subscriptionClient.modifyAckDeadline(subscriptionName, Arrays.asList(message.getAckId()),
                        0);
            }
        }
        if (notFoundMessage) {
            throw new RuntimeException("Log de mensajes: [" + log + "]. Message to consume not found");
        }
        return "OK";
    }

}
