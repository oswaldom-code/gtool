package launcher.pubsub.operations;

import launcher.pubsub.PubSub.PubSubData;

public interface PubSubOperation {

    Integer MAX_REQUESTED_MESSAGES = 100;

    String execute(PubSubData pubSubData) throws Exception;
}
