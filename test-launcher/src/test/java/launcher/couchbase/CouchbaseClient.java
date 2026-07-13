package launcher.couchbase;

import java.util.Map;

import com.couchbase.client.java.Bucket;
import com.couchbase.client.java.Cluster;
import com.couchbase.client.java.Collection;
import com.couchbase.client.java.Scope;
import com.couchbase.client.java.kv.GetResult;
import com.couchbase.client.java.query.QueryResult;
import com.jayway.jsonpath.Configuration;
import com.jayway.jsonpath.JsonPath;

/**
 * Couchbase helper for reads and N1QL queries against a running cluster.
 * Exposed to Karate via Java.type.
 */
public class CouchbaseClient {

    private final Bucket bucket;

    /**
     * @param config {@code connectionString}, {@code bucketName}, {@code user}, {@code password}.
     */
    public CouchbaseClient(final Map<String, Object> config) {
        final String connectionString = (String) config.get("connectionString");
        final String bucketName = (String) config.get("bucketName");
        final String user = (String) config.get("user");
        final String password = (String) config.get("password");

        final Cluster cluster = Cluster.connect(connectionString, user, password);
        this.bucket = cluster.bucket(bucketName);
    }

    /** Read a JSONPath field from a document. */
    public Object getField(final String scopeName, final String collectionName, final String id, final String field) {
        final Collection collection = bucket.scope(scopeName).collection(collectionName);
        final Object json = Configuration.defaultConfiguration().jsonProvider()
                .parse(collection.get(id).contentAsObject().toString());
        return JsonPath.read(json, field);
    }

    /** Read a whole document as a JSON string. */
    public String getDocument(final String scopeName, final String collectionName, final String id) {
        final Collection collection = bucket.scope(scopeName).collection(collectionName);
        final GetResult getResult = collection.get(id);
        return getResult.contentAsObject().toString();
    }

    /** Whether a document exists. */
    public Boolean documentExists(final String scopeName, final String collectionName, final String id) {
        final Collection collection = bucket.scope(scopeName).collection(collectionName);
        return collection.exists(id).exists();
    }

    /** Run a scoped N1QL query, returning the rows as a JSON string (or the error message). */
    public String executeQuery(final String query, final String bucketName, final String scopeName) {
        final Scope scope = bucket.scope(scopeName);
        try {
            final QueryResult result = scope.query(query);
            return result.rowsAsObject().toString();
        } catch (Exception e) {
            return e.getMessage();
        }
    }
}
