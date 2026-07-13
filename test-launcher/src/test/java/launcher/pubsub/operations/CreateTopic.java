package launcher.pubsub.operations;

import com.google.pubsub.v1.Topic;
import launcher.pubsub.PubSub.PubSubData;

import java.io.IOException;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class CreateTopic implements PubSubOperation {

    private final Logger logger = LoggerFactory.getLogger(CreateTopic.class);

    public String execute(PubSubData pubSubData) throws IOException {
        Topic topic = pubSubData.topicClient.createTopic(pubSubData.topicName);
        logger.info("Created topic: {}", topic.getName());
        return "OK";
    }
}
