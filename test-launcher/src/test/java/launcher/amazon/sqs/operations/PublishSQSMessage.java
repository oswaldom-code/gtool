package launcher.amazon.sqs.operations;

import java.net.URI;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import software.amazon.awssdk.auth.credentials.AwsBasicCredentials;
import software.amazon.awssdk.auth.credentials.StaticCredentialsProvider;
import software.amazon.awssdk.regions.Region;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.GetQueueUrlRequest;
import software.amazon.awssdk.services.sqs.model.SendMessageRequest;
import software.amazon.awssdk.services.sqs.model.SqsException;

/**
 * Publishes a message to an SQS queue on a local endpoint (e.g. LocalStack).
 * Returns "OK"/"KO". Exposed to Karate via Java.type.
 */
public class PublishSQSMessage {

    private final Logger logger = LoggerFactory.getLogger(PublishSQSMessage.class);

    public static class SQSData {
        public String queueName;
        public String message;
    }

    private final SqsClient sqsClient = SqsClient.builder()
            .region(Region.US_EAST_1)
            .endpointOverride(URI.create("http://localhost:4566"))
            .credentialsProvider(StaticCredentialsProvider.create(
                    AwsBasicCredentials.builder()
                            .accountId("000000000000")
                            .accessKeyId("test")
                            .secretAccessKey("test")
                            .build()))
            .build();

    public String run(final SQSData sqsData) {
        try {
            final String queueUrl = sqsClient.getQueueUrl(
                    GetQueueUrlRequest.builder().queueName(sqsData.queueName).build()).queueUrl();

            sqsClient.sendMessage(SendMessageRequest.builder()
                    .queueUrl(queueUrl)
                    .messageBody(sqsData.message)
                    .build());
        } catch (SqsException e) {
            logger.error("Exception publishing message to SQS", e);
            return "KO";
        } finally {
            sqsClient.close();
        }
        return "OK";
    }
}
