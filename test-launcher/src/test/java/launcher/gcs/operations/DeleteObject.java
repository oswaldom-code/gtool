package launcher.gcs.operations;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.google.cloud.NoCredentials;
import com.google.cloud.storage.Blob;
import com.google.cloud.storage.Storage;
import com.google.cloud.storage.StorageException;
import com.google.cloud.storage.StorageOptions;

/**
 * Deletes an object from the local GCS emulator (fake-gcs-server). Returns
 * whether the object was deleted.
 */
public class DeleteObject implements GcsOperation {

    private static final String GCS_SERVER = "http://localhost:9086";
    private static final Logger logger = LoggerFactory.getLogger(DeleteObject.class);

    @Override
    public Object execute(final String bucket, final String filePath) {
        final Storage storage = StorageOptions.newBuilder()
                .setHost(GCS_SERVER)
                .setProjectId("test-project")
                .setCredentials(NoCredentials.getInstance())
                .build()
                .getService();

        final Blob blob = storage.get(bucket, filePath);
        if (blob == null) {
            logger.debug("Object [{}] not found in bucket [{}], nothing to delete", filePath, bucket);
            return false;
        }

        try {
            final boolean result = blob.delete();
            if (result) {
                logger.debug("Object [{}] deleted from bucket [{}]", filePath, bucket);
            } else {
                logger.debug("Object [{}] found in bucket [{}] but could not be deleted", filePath, bucket);
            }
            return result;
        } catch (StorageException e) {
            logger.error("Object [{}] found in bucket [{}] but could not be deleted", filePath, bucket, e);
            return false;
        }
    }
}
