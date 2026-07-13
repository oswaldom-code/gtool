package launcher.pubsub.operations;

import com.google.pubsub.v1.ProjectSubscriptionName;
import com.google.pubsub.v1.PushConfig;
import com.google.pubsub.v1.Subscription;
import com.google.pubsub.v1.TopicName;

import launcher.pubsub.PubSub.PubSubData;

import java.io.IOException;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class CreateSubscription implements PubSubOperation {

    private final Logger logger = LoggerFactory.getLogger(CreateSubscription.class);

    public String execute(PubSubData pubSubData) throws IOException {
        ProjectSubscriptionName subscriptionName = ProjectSubscriptionName.of(pubSubData.projectId,
                pubSubData.subscriptionId);
        // Create a pull subscription with default acknowledgement deadline of 10
        // seconds.
        // Messages not successfully acknowledged within 10 seconds will get resent by
        // the server.
        Subscription subscription = pubSubData.subscriptionClient.createSubscription(subscriptionName,
                TopicName.of(pubSubData.projectId, pubSubData.topicId), PushConfig.getDefaultInstance(), 10);
        logger.info("Created pull subscription: {}", subscription.getName());
        return "OK";
    }
}
