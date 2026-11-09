package launcher.gcs.operations;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.google.cloud.NoCredentials;
import com.google.cloud.storage.Storage;
import com.google.cloud.storage.StorageOptions;

import launcher.util.RunPorts;

/** Storage clients for the local GCS emulator (fake-gcs-server) on this run's port. */
final class GcsStorage {

    private static final String GCS_SERVER = "http://localhost:" + RunPorts.port("gcs", "4443");
    private static final Logger logger = LoggerFactory.getLogger(GcsStorage.class);

    private GcsStorage() {
    }

    static Storage open() {
        return StorageOptions.newBuilder()
                .setHost(GCS_SERVER)
                .setProjectId("test-project")
                .setCredentials(NoCredentials.getInstance())
                .build()
                .getService();
    }

    static void close(final Storage storage) {
        try {
            storage.close();
        } catch (Exception e) {
            logger.debug("error closing GCS storage client", e);
        }
    }
}
