package launcher.s3;

import java.net.URI;
import java.util.Map;

import software.amazon.awssdk.auth.credentials.AwsBasicCredentials;
import software.amazon.awssdk.auth.credentials.StaticCredentialsProvider;
import software.amazon.awssdk.core.ResponseBytes;
import software.amazon.awssdk.core.sync.RequestBody;
import software.amazon.awssdk.regions.Region;
import software.amazon.awssdk.services.s3.S3Configuration;
import software.amazon.awssdk.services.s3.model.DeleteObjectRequest;
import software.amazon.awssdk.services.s3.model.GetObjectRequest;
import software.amazon.awssdk.services.s3.model.GetObjectResponse;
import software.amazon.awssdk.services.s3.model.HeadObjectRequest;
import software.amazon.awssdk.services.s3.model.NoSuchKeyException;
import software.amazon.awssdk.services.s3.model.PutObjectRequest;

/**
 * S3-compatible object storage helper (AWS S3 or MinIO via endpoint override),
 * for seeding and asserting objects. Exposed to Karate via Java.type.
 */
public class S3Client {

    private final software.amazon.awssdk.services.s3.S3Client s3;

    /**
     * @param config {@code endpoint} (optional, e.g. http://localhost:9000 for MinIO),
     *               {@code region} (default us-east-1), {@code accessKey}, {@code secretKey}.
     *               Path-style access is enabled for MinIO compatibility.
     */
    public S3Client(final Map<String, Object> config) {
        final String region = config.getOrDefault("region", "us-east-1").toString();
        final String accessKey = config.getOrDefault("accessKey", "test").toString();
        final String secretKey = config.getOrDefault("secretKey", "test").toString();

        final var builder = software.amazon.awssdk.services.s3.S3Client.builder()
                .region(Region.of(region))
                .credentialsProvider(StaticCredentialsProvider.create(
                        AwsBasicCredentials.create(accessKey, secretKey)))
                .serviceConfiguration(S3Configuration.builder().pathStyleAccessEnabled(true).build());

        final Object endpoint = config.get("endpoint");
        if (endpoint != null) {
            builder.endpointOverride(URI.create(endpoint.toString()));
        }
        this.s3 = builder.build();
    }

    /** Upload an object from a string body. */
    public void putObject(final String bucket, final String key, final String content) {
        s3.putObject(PutObjectRequest.builder().bucket(bucket).key(key).build(),
                RequestBody.fromString(content));
    }

    /** Download an object as bytes (null if missing). */
    public byte[] getObject(final String bucket, final String key) {
        try {
            final ResponseBytes<GetObjectResponse> response = s3.getObjectAsBytes(
                    GetObjectRequest.builder().bucket(bucket).key(key).build());
            return response.asByteArray();
        } catch (NoSuchKeyException e) {
            return null;
        }
    }

    /** Download an object as a UTF-8 string (null if missing). */
    public String getObjectAsString(final String bucket, final String key) {
        final byte[] bytes = getObject(bucket, key);
        return bytes != null ? new String(bytes) : null;
    }

    /** Whether an object exists. */
    public Boolean objectExists(final String bucket, final String key) {
        try {
            s3.headObject(HeadObjectRequest.builder().bucket(bucket).key(key).build());
            return true;
        } catch (NoSuchKeyException e) {
            return false;
        }
    }

    /** Delete an object. */
    public void deleteObject(final String bucket, final String key) {
        s3.deleteObject(DeleteObjectRequest.builder().bucket(bucket).key(key).build());
    }
}
