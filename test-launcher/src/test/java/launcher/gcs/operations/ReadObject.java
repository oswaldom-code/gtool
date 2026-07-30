package launcher.gcs.operations;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.nio.ByteBuffer;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.google.cloud.NoCredentials;
import com.google.cloud.ReadChannel;
import com.google.cloud.storage.Blob;
import com.google.cloud.storage.Storage;
import com.google.cloud.storage.StorageOptions;

/**
 * Reads an object from the local GCS emulator (fake-gcs-server). Returns the
 * bytes, or null if the object does not exist.
 */
public class ReadObject implements GcsOperation {

    private static final String GCS_SERVER = "http://localhost:9086";
    private static final Logger logger = LoggerFactory.getLogger(ReadObject.class);

    @Override
    public byte[] execute(final String bucket, final String filePath) {
        final Storage storage = StorageOptions.newBuilder()
                .setHost(GCS_SERVER)
                .setProjectId("test-project")
                .setCredentials(NoCredentials.getInstance())
                .build()
                .getService();

        final Blob blob = storage.get(bucket, filePath);
        if (blob == null) {
            logger.debug("Object [{}] not found in bucket [{}]", filePath, bucket);
            return null;
        }

        logger.debug("Object [{}] found in bucket [{}]", filePath, bucket);
        try (ReadChannel readChannel = blob.reader()) {
            final ByteArrayOutputStream outputStream = new ByteArrayOutputStream();
            final byte[] buffer = new byte[1024];
            int bytesRead;
            while ((bytesRead = readChannel.read(ByteBuffer.wrap(buffer))) != -1) {
                outputStream.write(buffer, 0, bytesRead);
            }
            return outputStream.toByteArray();
        } catch (IOException e) {
            logger.error("Error reading object [{}] in bucket [{}]", filePath, bucket, e);
            return null;
        }
    }
}
