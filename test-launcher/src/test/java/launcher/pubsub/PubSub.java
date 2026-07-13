package launcher.pubsub;

import java.util.List;
import java.util.Map;

import com.google.api.client.util.Strings;
import com.google.api.gax.core.CredentialsProvider;
import com.google.api.gax.core.NoCredentialsProvider;
import com.google.api.gax.grpc.GrpcTransportChannel;
import com.google.api.gax.rpc.FixedTransportChannelProvider;
import com.google.api.gax.rpc.TransportChannelProvider;
import com.google.cloud.pubsub.v1.Publisher;
import com.google.cloud.pubsub.v1.SubscriptionAdminClient;
import com.google.cloud.pubsub.v1.SubscriptionAdminSettings;
import com.google.cloud.pubsub.v1.TopicAdminClient;
import com.google.cloud.pubsub.v1.TopicAdminSettings;
import com.google.pubsub.v1.TopicName;

import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import launcher.pubsub.operations.PubSubOperation;

/**
 * Runs Pub/Sub operations against the local emulator (host:port from
 * {@code PUBSUB_EMULATOR_HOST}). Exposed to Karate as {@code Java.type('launcher.pubsub.PubSub')}.
 */
public class PubSub {

    public static class PubSubData {
        public String projectId;
        public String subscriptionId;
        public String topicId;
        public String message;
        public String expectedMessage;
        public String ceType;
        public String ceSource;
        public List<String> ignoreMessageFields;
        public List<String> orderedMessages;
        public String orderingKey;
        public Integer maxRequestedMessages;
        public Integer timeoutConsumeMessageSeconds;
        public Map<String, String> attributes;
        public TopicName topicName;
        public TopicAdminClient topicClient;
        public SubscriptionAdminClient subscriptionClient;
        public Publisher publisher;
        public TransportChannelProvider channelProvider;
    }

    /**
     * Runs an operation, propagating any exception (use in debug/strict scenarios).
     */
    public String runOperationOnEmulatorDebug(final PubSubOperation operation, final PubSubData data) throws Exception {
        final String hostport = System.getenv("PUBSUB_EMULATOR_HOST");
        final ManagedChannel channel = ManagedChannelBuilder.forTarget(hostport).usePlaintext().build();
        try {
            wire(data, channel);
            return operation.execute(data);
        } finally {
            channel.shutdown();
        }
    }

    /**
     * Runs an operation, returning "KO" on any failure.
     */
    public String runOperationOnEmulator(final PubSubOperation operation, final PubSubData data) throws Exception {
        final String hostport = System.getenv("PUBSUB_EMULATOR_HOST");
        final ManagedChannel channel = ManagedChannelBuilder.forTarget(hostport).usePlaintext().build();
        try {
            wire(data, channel);
            return operation.execute(data);
        } catch (Exception e) {
            return "KO";
        } finally {
            channel.shutdown();
        }
    }

    private void wire(final PubSubData data, final ManagedChannel channel) throws Exception {
        final TransportChannelProvider channelProvider =
                FixedTransportChannelProvider.create(GrpcTransportChannel.create(channel));
        final CredentialsProvider credentialsProvider = NoCredentialsProvider.create();

        data.topicClient = TopicAdminClient.create(TopicAdminSettings.newBuilder()
                .setTransportChannelProvider(channelProvider)
                .setCredentialsProvider(credentialsProvider).build());

        data.subscriptionClient = SubscriptionAdminClient.create(SubscriptionAdminSettings.newBuilder()
                .setTransportChannelProvider(channelProvider)
                .setCredentialsProvider(credentialsProvider).build());

        data.topicName = TopicName.of(data.projectId, data.topicId);

        data.publisher = Publisher.newBuilder(data.topicName)
                .setChannelProvider(channelProvider)
                .setCredentialsProvider(credentialsProvider)
                .setEnableMessageOrdering(!Strings.isNullOrEmpty(data.orderingKey)).build();

        data.channelProvider = channelProvider;
    }
}
