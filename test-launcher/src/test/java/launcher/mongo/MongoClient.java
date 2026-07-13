package launcher.mongo;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import org.bson.Document;

import com.mongodb.client.MongoCollection;
import com.mongodb.client.MongoDatabase;
import com.mongodb.client.MongoClients;

/**
 * MongoDB helper for seeding and asserting documents against a running MongoDB.
 * Filters and documents are passed as JSON strings. Exposed to Karate via Java.type.
 */
public class MongoClient {

    private final com.mongodb.client.MongoClient client;
    private final MongoDatabase database;

    /**
     * @param config {@code connectionString} (e.g. mongodb://localhost:27017), {@code database}.
     */
    public MongoClient(final Map<String, Object> config) {
        final String connectionString = (String) config.get("connectionString");
        final String databaseName = (String) config.get("database");
        this.client = MongoClients.create(connectionString);
        this.database = client.getDatabase(databaseName);
    }

    private MongoCollection<Document> collection(final String name) {
        return database.getCollection(name);
    }

    /** Insert a document from a JSON string. Returns the inserted JSON. */
    public String insert(final String collection, final String json) {
        final Document doc = Document.parse(json);
        collection(collection).insertOne(doc);
        return doc.toJson();
    }

    /** Find all documents matching a JSON filter. */
    public List<String> find(final String collection, final String filterJson) {
        final List<String> results = new ArrayList<>();
        for (final Document doc : collection(collection).find(Document.parse(filterJson))) {
            results.add(doc.toJson());
        }
        return results;
    }

    /** Find the first document matching a JSON filter (null if none). */
    public String findOne(final String collection, final String filterJson) {
        final Document doc = collection(collection).find(Document.parse(filterJson)).first();
        return doc != null ? doc.toJson() : null;
    }

    /** Whether at least one document matches the JSON filter. */
    public Boolean exists(final String collection, final String filterJson) {
        return collection(collection).countDocuments(Document.parse(filterJson)) > 0;
    }

    /** Delete documents matching the JSON filter. Returns the deleted count. */
    public Long delete(final String collection, final String filterJson) {
        return collection(collection).deleteMany(Document.parse(filterJson)).getDeletedCount();
    }
}
