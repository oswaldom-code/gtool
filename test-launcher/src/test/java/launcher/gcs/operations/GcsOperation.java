package launcher.gcs.operations;

public interface GcsOperation {
    Object execute(String bucket, String filePath);
}
